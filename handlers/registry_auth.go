package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/docker/distribution/reference"
)

type DockerConfig struct {
	Auths map[string]DockerRegistryAuthConfig `json:"auths"`
}

type DockerRegistryAuthConfig struct {
	Auth string `json:"auth"`
}

func DefaultDockerConfig() DockerConfig {
	return DockerConfig{
		Auths: map[string]DockerRegistryAuthConfig{},
	}
}

func ParseDockerConfig() (DockerConfig, error) {
	dockerConfigPath, foundDockerConfigPath := os.LookupEnv("DOCKER_CONFIG_PATH")
	if !foundDockerConfigPath {
		dockerConfigPath = "/run/secrets/docker-config"
	}

	if _, err := os.Stat(dockerConfigPath); errors.Is(err, os.ErrNotExist) {
		log.Printf("did not find docker-config secret file at " + dockerConfigPath)
		return DefaultDockerConfig(), nil
	}

	// Open our jsonFile
	jsonFile, err := os.Open(dockerConfigPath)
	// if we os.Open returns an error then handle it
	if err != nil {
		fmt.Println(err)
		return DefaultDockerConfig(), err
	}

	defer jsonFile.Close()

	byteValue, _ := ioutil.ReadAll(jsonFile)
	var dockerConfig DockerConfig

	err = json.Unmarshal(byteValue, &dockerConfig)
	if err != nil {
		log.Printf("Error parsing docker config %s", err)
		return DefaultDockerConfig(), err
	}

	log.Printf("found auth for %d registries in path %s", len(dockerConfig.Auths), dockerConfigPath)

	return dockerConfig, nil
}

func GetAuthFromImage(dockerConfig DockerConfig, image string) (string, error) {
	log.Printf("attempting to get auth for docker image %s", image)

	named, err := reference.ParseNamed(image)
	if err != nil {
		log.Printf("failed parsing image reference %s", image)
		return "", err
	}

	domain := reference.Domain(named)
	if domain == "" {
		log.Printf("No domain detected on docker image %s. Using no special host for credential lookup", image)
	}

	log.Printf("detected registry %s, looking up credentials...", domain)
	return GetAuth(dockerConfig, domain)
}

func GetAuth(dockerConfig DockerConfig, registry string) (string, error) {

	if dockerConfig.Auths != nil {
		dockerRegistryConfig, exist := dockerConfig.Auths[registry]
		if exist {
			log.Printf("found docker registry auth for registry %s", registry)
			return dockerRegistryConfig.Auth, nil
		}
	}

	log.Printf("did not find auth for registry %s", registry)

	return "", nil
}
