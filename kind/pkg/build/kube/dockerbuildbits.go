/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package kube

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	"k8s.io/test-infra/kind/pkg/exec"
)

// TODO(bentheelder): plumb through arch

// DockerBuildBits implements Bits for a local docker-ized make / bash build
type DockerBuildBits struct {
	kubeRoot string
}

var _ Bits = &DockerBuildBits{}

func init() {
	RegisterNamedBits("docker", NewDockerBuildBits)
	RegisterNamedBits("make", NewDockerBuildBits)
}

// NewDockerBuildBits returns a new Bits backed by the docker-ized build,
// given kubeRoot, the path to the kubernetes source directory
func NewDockerBuildBits(kubeRoot string) (bits Bits, err error) {
	return &DockerBuildBits{
		kubeRoot: kubeRoot,
	}, nil
}

// Build implements Bits.Build
func (b *DockerBuildBits) Build() error {
	// cd to k8s source
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	os.Chdir(b.kubeRoot)
	// make sure we cd back when done
	defer os.Chdir(cwd)

	// the PR that added `make quick-release-images` added this script,
	// we can use it's presence to detect if that target exists
	// TODO(bentheelder): drop support for building without this once we've
	// dropped older releases or gotten support for `make quick-release-iamges`
	// back ported to them ...
	releaseImagesSH := filepath.Join(
		b.kubeRoot, "build", "release-images.sh",
	)
	// if we can't find the script, use the non `make quick-release-images` build
	if _, err := os.Stat(releaseImagesSH); err != nil {
		if err := b.buildBash(); err != nil {
			return err
		}
	} else {
		// otherwise leverage `make quick-release-images`
		if err := b.build(); err != nil {
			return err
		}
	}

	// capture version info
	return buildVersionFile(b.kubeRoot)
}

// binary and image build when we have `make quick-release-images` support
func (b *DockerBuildBits) build() error {
	// build binaries
	cmd := exec.Command("build/run.sh", "make", "all")
	what := []string{
		// binaries we use directly
		"cmd/kubeadm",
		"cmd/kubectl",
		"cmd/kubelet",
	}
	cmd.Args = append(cmd.Args,
		"WHAT="+strings.Join(what, " "),
		"KUBE_BUILD_PLATFORMS=linux/amd64",
		// ensure the build isn't especially noisy..
		"KUBE_VERBOSE=0",
	)
	cmd.Env = append(cmd.Env, os.Environ()...)
	cmd.Debug = true
	cmd.InheritOutput = true
	if err := cmd.Run(); err != nil {
		return errors.Wrap(err, "failed to build binaries")
	}

	// build images
	cmd = exec.Command("make", "quick-release-images", "KUBE_BUILD_HYPERKUBE=n")
	cmd.Env = append(cmd.Env, os.Environ()...)
	cmd.Debug = true
	cmd.InheritOutput = true
	if err := cmd.Run(); err != nil {
		return errors.Wrap(err, "failed to build images")
	}
	return nil
}

// binary and image build when we don't have `make quick-release-images` support
func (b *DockerBuildBits) buildBash() error {
	// build binaries
	cmd := exec.Command("build/run.sh", "make", "all")
	what := []string{
		// binaries we use directly
		"cmd/kubeadm",
		"cmd/kubectl",
		"cmd/kubelet",
		// docker image wrapped binaries
		"cmd/cloud-controller-manager",
		"cmd/kube-apiserver",
		"cmd/kube-controller-manager",
		"cmd/kube-scheduler",
		"cmd/kube-proxy",
		// we don't need this one, but the image build wraps it...
		"vendor/k8s.io/kube-aggregator",
	}
	cmd.Args = append(cmd.Args,
		"WHAT="+strings.Join(what, " "), "KUBE_BUILD_PLATFORMS=linux/amd64",
	)
	cmd.Env = append(cmd.Env, os.Environ()...)
	// ensure the build isn't especially noisy..
	cmd.Env = append(cmd.Env, "KUBE_VERBOSE=0")
	cmd.Debug = true
	cmd.InheritOutput = true
	if err := cmd.Run(); err != nil {
		return errors.Wrap(err, "failed to build binaries")
	}

	// mimic `make quick-release` internals, clear previous images
	if err := os.RemoveAll(filepath.Join(
		".", "_output", "release-images", "amd64",
	)); err != nil {
		return errors.Wrap(err, "failed to remove old release-images")
	}

	// build images
	buildImages := []string{
		"source build/common.sh;",
		"source hack/lib/version.sh;",
		"source build/lib/release.sh;",
		"kube::version::get_version_vars;",
		`kube::release::create_docker_images_for_server "${LOCAL_OUTPUT_ROOT}/dockerized/bin/linux/amd64" "amd64"`,
	}
	cmd = exec.Command("bash", "-c", strings.Join(buildImages, " "))
	cmd.Env = append(cmd.Env, os.Environ()...)
	cmd.Env = append(cmd.Env, "KUBE_BUILD_HYPERKUBE=n")
	cmd.Debug = true
	cmd.InheritOutput = true
	if err := cmd.Run(); err != nil {
		return errors.Wrap(err, "failed to build images")
	}
	return nil
}

// Paths implements Bits.Paths
func (b *DockerBuildBits) Paths() map[string]string {
	binDir := filepath.Join(b.kubeRoot,
		"_output", "dockerized", "bin", "linux", "amd64",
	)
	imageDir := filepath.Join(b.kubeRoot,
		"_output", "release-images", "amd64",
	)
	return map[string]string{
		// binaries (hyperkube)
		filepath.Join(binDir, "kubeadm"): "bin/kubeadm",
		filepath.Join(binDir, "kubelet"): "bin/kubelet",
		filepath.Join(binDir, "kubectl"): "bin/kubectl",
		// docker images
		filepath.Join(imageDir, "kube-apiserver.tar"):          "images/kube-apiserver.tar",
		filepath.Join(imageDir, "kube-controller-manager.tar"): "images/kube-controller-manager.tar",
		filepath.Join(imageDir, "kube-scheduler.tar"):          "images/kube-scheduler.tar",
		filepath.Join(imageDir, "kube-proxy.tar"):              "images/kube-proxy.tar",
		// version file
		filepath.Join(b.kubeRoot, "_output", "git_version"): "version",
		// borrow kubelet service files from bazel debians
		// TODO(bentheelder): probably we should use our own config instead :-)
		filepath.Join(b.kubeRoot, "build", "debs", "kubelet.service"): "systemd/kubelet.service",
		filepath.Join(b.kubeRoot, "build", "debs", "10-kubeadm.conf"): "systemd/10-kubeadm.conf",
	}
}

// Install implements Bits.Install
func (b *DockerBuildBits) Install(install InstallContext) error {
	kindBinDir := path.Join(install.BasePath(), "bin")

	// symlink the kubernetes binaries into $PATH
	binaries := []string{"kubeadm", "kubelet", "kubectl"}
	for _, binary := range binaries {
		if err := install.Run("ln", "-s",
			path.Join(kindBinDir, binary),
			path.Join("/usr/bin/", binary),
		); err != nil {
			return errors.Wrap(err, "failed to symlink binaries")
		}
	}

	// enable the kubelet service
	kubeletService := path.Join(install.BasePath(), "systemd/kubelet.service")
	if err := install.Run("systemctl", "enable", kubeletService); err != nil {
		return errors.Wrap(err, "failed to enable kubelet service")
	}

	// setup the kubelet dropin
	kubeletDropinSource := path.Join(install.BasePath(), "systemd/10-kubeadm.conf")
	kubeletDropin := "/etc/systemd/system/kubelet.service.d/10-kubeadm.conf"
	if err := install.Run("mkdir", "-p", path.Dir(kubeletDropin)); err != nil {
		return errors.Wrap(err, "failed to configure kubelet service")
	}
	if err := install.Run("cp", kubeletDropinSource, kubeletDropin); err != nil {
		return errors.Wrap(err, "failed to configure kubelet service")
	}

	return nil
}
