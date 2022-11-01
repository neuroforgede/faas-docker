package handlers

import (
	swarmTypes "github.com/docker/docker/api/types/swarm"
)

func isFunctionAndPartOfProject(service swarmTypes.Service) bool {
	isFunction := len(service.Spec.TaskTemplate.ContainerSpec.Labels["function"]) > 0 && len(service.Spec.TaskTemplate.ContainerSpec.Labels["com.openfaas.function"]) > 0
	isPartOfProject := service.Spec.TaskTemplate.ContainerSpec.Labels["com.github.neuroforgede.nf-faas-docker.project"] == globalConfig.NFFaaSDockerProject
	return isFunction && isPartOfProject
}
