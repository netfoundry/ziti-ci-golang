/*
 * Copyright NetFoundry, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

type publishToArtifactoryCmd struct {
	BaseCommand
}

type artifact struct {
	name            string
	artifactArchive string
	sourceName      string
	sourcePath      string
	artifactPath    string
	arch            string
	os              string
}

func (cmd *publishToArtifactoryCmd) Execute() {
	jfrogApiKey, found := os.LookupEnv("JFROG_API_KEY")
	if !found {
		cmd.Failf("JFROG_API_KEY not specified")
	}

	cmd.EvalCurrentAndNextVersion()

	releaseDir, err := filepath.Abs("./release")
	cmd.exitIfErrf(err, "could not get absolute path for releases directory")

	archDirs, err := ioutil.ReadDir(releaseDir)
	cmd.exitIfErrf(err, "failed to read releases dir: %v\n", err)
	var artifacts []*artifact
	for _, archDir := range archDirs {
		arch := archDir.Name()
		cmd.Infof("processing files for arch: %v\n", arch)
		archDirPath := filepath.Join(releaseDir, archDir.Name())

		if archDir.IsDir() {
			osDirs, err := ioutil.ReadDir(archDirPath)
			cmd.exitIfErrf(err, "failed to read arch dir %v: %v\n", archDirPath, err)

			for _, osDir := range osDirs {
				os := osDir.Name()
				cmd.Infof("processing files for: %v/%v\n", arch, os)

				osDirPath := filepath.Join(archDirPath, osDir.Name())
				releasableFiles, err := ioutil.ReadDir(osDirPath)
				cmd.exitIfErrf(err, "failed to read os dir %v: %v\n", osDirPath, err)

				for _, releasableFile := range releasableFiles {
					if !releasableFile.IsDir() && !strings.HasSuffix(releasableFile.Name(), ".gz") {
						name := releasableFile.Name()
						if strings.HasSuffix(name, ".exe") {
							name = strings.TrimSuffix(name, ".exe")
						}
						filePath := filepath.Join(osDirPath, releasableFile.Name())
						destPath := filepath.Join(osDirPath, name+".tar.gz")
						cmd.Infof("packaging releasable: %v -> %v\n", filePath, destPath)
						cmd.tarGzSimple(destPath, filePath)
						artifacts = append(artifacts, &artifact{
							name:            name,
							sourceName:      releasableFile.Name(),
							sourcePath:      filePath,
							artifactArchive: name + ".tar.gz",
							artifactPath:    destPath,
							arch:            arch,
							os:              os,
						})
					}
				}
			}
		}
	}

	zitiAllPath := "release/ziti-all.tar.gz"
	cmd.tarGzArtifacts(zitiAllPath, artifacts...)

	// When rolling minor/major numbers the current version will be nil, so use the next version instead
	// This will only happen when publishing a PR
	version := cmd.getPublishVersion().String()
	if !cmd.isReleaseBranch() {
		version = fmt.Sprintf("%v-%v", version, cmd.getBuildNumber())
	}

	for _, artifact := range artifacts {
		dest := ""
		// if release branch, publish to staging, otherwise to snapshot
		if cmd.isReleaseBranch() {
			dest = fmt.Sprintf("ziti-staging/%v/%v/%v/%v/%v",
				artifact.name, artifact.arch, artifact.os, version, artifact.artifactArchive)
		} else {
			dest = fmt.Sprintf("ziti-snapshot/%v/%v/%v/%v/%v/%v",
				cmd.GetCurrentBranch(), artifact.name, artifact.arch, artifact.os, version, artifact.artifactArchive)
		}
		props := fmt.Sprintf("version=%v;name=%v;arch=%v;os=%v;branch=%v", version, artifact.name, artifact.arch, artifact.os, cmd.GetCurrentBranch())
		cmd.runCommand(fmt.Sprintf("Publish artifact for %v", artifact.name),
			"jfrog-cli", "rt", "u", artifact.artifactPath, dest,
			"--apikey", jfrogApiKey,
			"--url", "https://netfoundry.jfrog.io/netfoundry",
			"--props", props,
			"--build-name=ziti",
			"--build-number="+cmd.getPublishVersion().String())
	}

	if cmd.isReleaseBranch() {
		dest := fmt.Sprintf("ziti-staging/ziti-all/%v/ziti-all.%v.tar.gz", version, version)
		props := fmt.Sprintf("version=%v;branch=%v", version, cmd.GetCurrentBranch())
		cmd.runCommand("Publish artifact for ziti-all",
			"jfrog-cli", "rt", "u", zitiAllPath, dest,
			"--apikey", jfrogApiKey,
			"--url", "https://netfoundry.jfrog.io/netfoundry",
			"--props", props,
			"--build-name=ziti",
			"--build-number="+cmd.getPublishVersion().String())

		cmd.runCommand("Set build version", "jfrog-cli", "rt", "bce", "ziti", version)
		cmd.runCommand("Create build in Artifactory", "jfrog-cli", "rt", "bp",
			"--apikey", jfrogApiKey, "--url", "https://netfoundry.jfrog.io/netfoundry", "ziti", version)
	}
}

func newPublishToArtifactoryCmd(root *RootCommand) *cobra.Command {
	cobraCmd := &cobra.Command{
		Use:   "publish-to-artifactory",
		Short: "Publishes an artifact to artifactory",
		Args:  cobra.ExactArgs(0),
	}

	result := &publishToArtifactoryCmd{
		BaseCommand: BaseCommand{
			RootCommand: root,
			Cmd:         cobraCmd,
		},
	}

	return Finalize(result)
}
