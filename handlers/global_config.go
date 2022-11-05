package handlers

import (
	"os"

	types "github.com/neuroforgede/nf-faas-docker/types"
)

var globalConfig = types.GlobalConfig{
	NFFaaSDockerProject: "default",
	AlwaysResolveImage:  true,
}

const ProjectLabel = "com.github.neuroforgede.nf-faas-docker.project"
const AdditionalNetworksLabel = "com.github.neuroforgede.nf-faas-docker.additionalNetworks"

func InitGlobalConfig() {
	nfFaaSDockerProject, found := os.LookupEnv("NF_FAAS_DOCKER_PROJECT")
	if !found {
		panic("did not find NF_FAAS_DOCKER_PROJECT env var. Exiting to protect against cross project contamination")
	}
	globalConfig.NFFaaSDockerProject = nfFaaSDockerProject

	alwaysResolveImage, found := os.LookupEnv("NF_FAAS_ALWAYS_RESOLVE_IMAGE")
	if found {
		globalConfig.AlwaysResolveImage = alwaysResolveImage == "true"
	}
}

func GetGlobalConfig() types.GlobalConfig {
	return globalConfig
}

func ProjectSpecificName(name string) string {
	return globalConfig.NFFaaSDockerProject + "_" + name
}
