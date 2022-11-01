package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"
)

type DockerConfig struct {
	auths map[string]DockerRegistryAuthConfig
}

type DockerRegistryAuthConfig struct {
	auth string
}

func GetAuthFromImage(image string) (string, error) {
	url, err := url.Parse(image)
	if err != nil {
		return "", err
	}

	registry := url.Host
	return GetAuth(registry)
}

func GetAuth(registry string) (string, error) {
	dockerConfigPath, foundDockerConfigPath := os.LookupEnv("DOCKER_CONFIG_PATH")
	if !foundDockerConfigPath {
		dockerConfigPath = "/run/secret/docker-config"
	}

	if _, err := os.Stat(dockerConfigPath); errors.Is(err, os.ErrNotExist) {
		log.Println("did not find docker-config secret file at " + dockerConfigPath)
		return "", nil
	}

	// Open our jsonFile
	jsonFile, err := os.Open(dockerConfigPath)
	// if we os.Open returns an error then handle it
	if err != nil {
		fmt.Println(err)
		return "", err
	}

	defer jsonFile.Close()

	// read our opened xmlFile as a byte array.
	byteValue, _ := ioutil.ReadAll(jsonFile)

	// we initialize our Users array
	var dockerConfig DockerConfig

	// we unmarshal our byteArray which contains our
	// jsonFile's content into 'users' which we defined above
	err = json.Unmarshal(byteValue, &dockerConfig)
	if err != nil {
		fmt.Println(err)
		return "", err
	}

	if dockerConfig.auths != nil {
		dockerRegistryConfig, exist := dockerConfig.auths[registry]
		if exist {
			fmt.Println("detected docker registry auth", dockerRegistryConfig)
			return dockerRegistryConfig.auth, nil
		}
	}

	return "", nil
}
