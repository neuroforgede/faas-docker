package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/mount"

	"github.com/docker/distribution/reference"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/client"
	"github.com/docker/docker/registry"
	units "github.com/docker/go-units"
	typesv1 "github.com/openfaas/faas-provider/types"
)

const annotationLabelPrefix = "com.openfaas.annotations."

var linuxOnlyConstraints = []string{"node.platform.os == linux"}

// DeployHandler creates a new function (service) inside the swarm network.
func DeployHandler(dockerConfig DockerConfig, c *client.Client, maxRestarts uint64, restartDelay time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		body, _ := ioutil.ReadAll(r.Body)

		request := typesv1.FunctionDeployment{}
		err := json.Unmarshal(body, &request)
		if err != nil {
			log.Println("Error parsing request:", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		options := types.ServiceCreateOptions{}
		options.QueryRegistry = globalConfig.AlwaysResolveImage

		registryAuth, err := GetAuthFromImage(dockerConfig, request.Image)
		if err != nil {
			log.Println("Error building registry auth configuration:", err)
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Failed to get auth from Image"))
			return
		}

		if len(registryAuth) > 0 {
			auth, err := BuildEncodedAuthConfig(registryAuth, request.Image)
			if err != nil {
				log.Println("Error building registry auth configuration:", err)
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("Invalid registry auth"))
				return
			}
			options.EncodedRegistryAuth = auth
		}

		secrets, err := makeSecretsArray(c, request.Secrets)
		if err != nil {
			log.Printf("Deployment error: %s\n", err)

			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Deployment error: " + err.Error()))
			return
		}

		networks, networkErr := lookupNetworks(c, request.Annotations)
		if networkErr != nil {
			log.Printf("Error querying networks: %s\n", networkErr)
			return
		}

		spec, err := makeSpec(&request, maxRestarts, restartDelay, secrets, networks)
		if err != nil {

			log.Printf("Error creating specification: %s\n", err)

			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Deployment error: " + err.Error()))
			return
		}

		response, err := c.ServiceCreate(context.Background(), spec, options)
		if err != nil {

			log.Printf("Error creating service: %s\n", err)

			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Deployment error: " + err.Error()))
			return
		}

		if len(response.Warnings) > 0 {
			log.Println(response.Warnings)
		}

		w.WriteHeader(http.StatusAccepted)
	}
}

func lookupNetworks(c *client.Client, annotations *map[string]string) ([]string, error) {
	networkFilters := filters.NewArgs()
	networkFilters.Add("label", "openfaas=true")
	networkFilters.Add("label", ProjectLabel+"="+globalConfig.NFFaaSDockerProject)
	networkListOptions := types.NetworkListOptions{
		Filters: networkFilters,
	}

	var foundNetworks []string
	if annotations != nil {
		if additionalNetworksLabelValue, exist := (*annotations)[AdditionalNetworksLabel]; exist {
			additionalSpecifiedNetworks := strings.Split(additionalNetworksLabelValue, ",")
			for _, network := range additionalSpecifiedNetworks {
				foundNetworks = append(foundNetworks, network)
			}
		}
	}

	projectNetworks, networkErr := c.NetworkList(context.Background(), networkListOptions)
	if networkErr != nil {
		return foundNetworks, nil
	}

	if len(projectNetworks) > 0 {
		for _, network := range projectNetworks {
			foundNetworks = append(foundNetworks, network.Name)
		}
		return foundNetworks, nil
	}

	return foundNetworks, nil
}

func makeSpec(request *typesv1.FunctionDeployment, maxRestarts uint64, restartDelay time.Duration, secrets []*swarm.SecretReference, networks []string) (swarm.ServiceSpec, error) {
	constraints := []string{}

	if request.Constraints != nil && len(request.Constraints) > 0 {
		constraints = request.Constraints
	} else {
		constraints = linuxOnlyConstraints
	}

	labels, err := buildLabels(request)
	if err != nil {
		nilSpec := swarm.ServiceSpec{}
		return nilSpec, err
	}

	resources := buildResources(request)

	var nets []swarm.NetworkAttachmentConfig
	for _, network := range networks {
		nets = append(nets, swarm.NetworkAttachmentConfig{
			Target: network,
			// required so that the gateway can directly call via function name
			// and if we need some kind of deduplication via suffixes we provide that as well
			Aliases: []string{
				request.Service,
				request.Service + "_" + globalConfig.NFFaaSDockerProject,
			},
		})
	}

	spec := swarm.ServiceSpec{
		Annotations: swarm.Annotations{
			Name:   ProjectSpecificName(request.Service),
			Labels: labels,
		},
		TaskTemplate: swarm.TaskSpec{
			RestartPolicy: &swarm.RestartPolicy{
				MaxAttempts: &maxRestarts,
				Condition:   swarm.RestartPolicyConditionAny,
				Delay:       &restartDelay,
			},
			ContainerSpec: &swarm.ContainerSpec{
				Image:    request.Image,
				Labels:   labels,
				Secrets:  secrets,
				ReadOnly: request.ReadOnlyRootFilesystem,
			},
			Networks:  nets,
			Resources: resources,
			Placement: &swarm.Placement{
				Constraints: constraints,
			},
		},
		Mode: swarm.ServiceMode{
			Replicated: &swarm.ReplicatedService{
				Replicas: getMinReplicas(request),
			},
		},
	}

	if request.ReadOnlyRootFilesystem {
		spec.TaskTemplate.ContainerSpec.Mounts = []mount.Mount{
			{
				Type:   mount.TypeTmpfs,
				Target: "/tmp",
			},
		}
	}

	// TODO: request.EnvProcess should only be set if it's not nil, otherwise we override anything in the Docker image already
	env := buildEnv(request.EnvProcess, request.EnvVars)

	if len(env) > 0 {
		spec.TaskTemplate.ContainerSpec.Env = env
	}

	return spec, nil
}

func buildEnv(envProcess string, envVars map[string]string) []string {
	var env []string
	if len(envProcess) > 0 {
		env = append(env, fmt.Sprintf("fprocess=%s", envProcess))
	}

	for k, v := range envVars {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	return env
}

// BuildEncodedAuthConfig parses the image name for a repository, user name, and image name
// If a repository is not included (ie: username/function-name), 'docker.io/' will be prepended
func BuildEncodedAuthConfig(basicAuthB64 string, dockerImage string) (string, error) {
	// use docker.io if no repository was included
	if len(strings.Split(dockerImage, "/")) < 3 {
		dockerImage = registry.DefaultNamespace + "/" + dockerImage
	}

	distributionRef, err := reference.ParseNamed(dockerImage)
	if err != nil {
		return "", err
	}

	repoInfo, err := registry.ParseRepositoryInfo(distributionRef)
	if err != nil {
		return "", err
	}

	// extract registry user & password
	user, password, err := userPasswordFromBasicAuth(basicAuthB64)
	if err != nil {
		return "", err
	}

	// build encoded registry auth config
	buf, err := json.Marshal(types.AuthConfig{
		Username:      user,
		Password:      password,
		ServerAddress: repoInfo.Index.Name,
	})
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(buf), nil
}

func userPasswordFromBasicAuth(basicAuthB64 string) (string, string, error) {
	c, err := base64.StdEncoding.DecodeString(basicAuthB64)
	if err != nil {
		return "", "", err
	}
	cs := string(c)
	s := strings.IndexByte(cs, ':')
	if s < 0 {
		return "", "", errors.New("Invalid basic auth")
	}
	return cs[:s], cs[s+1:], nil
}

func parseMemory(value string) (int64, error) {
	return units.RAMInBytes(value)
}

func parseCPU(value string) (int64, error) {
	v, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, err
	}

	return v, nil
}

func buildResources(request *typesv1.FunctionDeployment) *swarm.ResourceRequirements {
	var resources *swarm.ResourceRequirements

	if request.Requests != nil || request.Limits != nil {

		resources = &swarm.ResourceRequirements{}

		if request.Limits != nil {
			limits := &swarm.Limit{}
			valueSet := false

			if len(request.Limits.Memory) > 0 {
				memoryBytes, err := parseMemory(request.Limits.Memory)
				if err != nil {
					log.Printf("Error parsing memory limit: %T", err)
				} else {
					limits.MemoryBytes = memoryBytes
					valueSet = true
				}
			}

			if len(request.Limits.CPU) > 0 {
				nanoCPUs, err := parseCPU(request.Limits.CPU)
				if err != nil {
					log.Printf("Error parsing cpu limit: %T", err)
				} else {
					limits.NanoCPUs = nanoCPUs
					valueSet = true
				}
			}

			if valueSet {
				resources.Limits = limits
			}
		}

		if request.Requests != nil {
			reservations := &swarm.Resources{}
			valueSet := false

			if len(request.Requests.Memory) > 0 {
				memoryBytes, err := parseMemory(request.Requests.Memory)
				if err != nil {
					log.Printf("Error parsing memory reservations: %T", err)
				} else {
					reservations.MemoryBytes = memoryBytes
					valueSet = true
				}
			}

			if len(request.Requests.CPU) > 0 {
				nanoCPUs, err := parseCPU(request.Requests.CPU)
				if err != nil {
					log.Printf("Error parsing cpu reservations: %T", err)
				} else {
					reservations.NanoCPUs = nanoCPUs
					valueSet = true
				}
			}

			if valueSet {
				resources.Reservations = reservations
			}
		}

	}
	return resources
}

func getMinReplicas(request *typesv1.FunctionDeployment) *uint64 {
	replicas := uint64(1)

	if request.Labels != nil {
		if val, exists := (*request.Labels)["com.openfaas.scale.min"]; exists {
			value, err := strconv.Atoi(val)
			if err != nil {
				log.Println(err)
			}
			replicas = uint64(value)
		}
	}
	return &replicas
}

func buildLabels(request *typesv1.FunctionDeployment) (map[string]string, error) {
	labels := map[string]string{
		"com.openfaas.function": request.Service,
		"function":              "true", // backwards-compatible
		ProjectLabel:            globalConfig.NFFaaSDockerProject,
	}

	if request.Labels != nil {
		for k, v := range *request.Labels {
			labels[k] = v
		}
	}

	if request.Annotations != nil {
		for k, v := range *request.Annotations {
			key := fmt.Sprintf("%s%s", annotationLabelPrefix, k)
			if _, ok := labels[key]; !ok {
				labels[key] = v
			} else {
				return nil, errors.New(fmt.Sprintf("Keys %s can not be used as a labels as is clashes with annotations", k))
			}
		}
	}

	return labels, nil
}
