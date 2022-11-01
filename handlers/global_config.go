package handlers

import (
	"os"

	types "github.com/neuroforgede/nf-faas-docker/types"
)

var globalConfig = types.GlobalConfig{
	NFFaaSDockerProject: "default",
}

func InitGlobalConfig() {
	nfFaaSDockerProject, found := os.LookupEnv("NF_FAAS_DOCKER_PROJECT")
	if !found {
		panic("did not find NF_FAAS_DOCKER_PROJECT env var. Exiting to protect against cross project contamination")
	}
	globalConfig.NFFaaSDockerProject = nfFaaSDockerProject
}

func GetGlobalConfig() types.GlobalConfig {
	return globalConfig
}
