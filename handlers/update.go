package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	types "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/client"

	typesv1 "github.com/openfaas/faas-provider/types"
)

// UpdateHandler updates an existng function
func UpdateHandler(dockerConfig DockerConfig, c *client.Client, maxRestarts uint64, restartDelay time.Duration) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()

		defer r.Body.Close()
		body, _ := ioutil.ReadAll(r.Body)

		request := typesv1.FunctionDeployment{}
		err := json.Unmarshal(body, &request)
		if err != nil {
			log.Println("Error parsing request:", err)
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}

		serviceInspectopts := types.ServiceInspectOptions{
			InsertDefaults: true,
		}

		service, _, err := c.ServiceInspectWithRaw(ctx, globalConfig.NFFaaSDockerProject+"_"+request.Service, serviceInspectopts)
		if err != nil {
			log.Println("Error inspecting service", err)
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(err.Error()))
			return
		}

		secrets, err := makeSecretsArray(c, request.Secrets)
		if err != nil {
			log.Println(err)
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Deployment error: " + err.Error()))
			return
		}

		// FIXME: add ability to specify network back (maybe via annotation?)
		networkValue, networkErr := lookupNetwork(c)
		if networkErr != nil {
			log.Printf("Error querying networks: %s\n", networkErr)
			return
		}

		if err := updateSpec(&request, &service.Spec, maxRestarts, restartDelay, secrets, networkValue); err != nil {
			log.Println("Error updating service spec:", err)
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Update spc error: " + err.Error()))
			return
		}

		updateOpts := types.ServiceUpdateOptions{}
		updateOpts.RegistryAuthFrom = types.RegistryAuthFromSpec

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
			updateOpts.EncodedRegistryAuth = auth
		}

		service.Spec.UpdateConfig.Order = "start-first"

		response, err := c.ServiceUpdate(ctx, service.ID, service.Version, service.Spec, updateOpts)

		if err != nil {
			log.Println("Error updating service:", err)
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Update error: " + err.Error()))
			return
		}

		if response.Warnings != nil {
			log.Println(response.Warnings)
		}

		w.WriteHeader(http.StatusAccepted)
	}
}

func updateSpec(request *typesv1.FunctionDeployment, spec *swarm.ServiceSpec, maxRestarts uint64, restartDelay time.Duration, secrets []*swarm.SecretReference, network string) error {

	constraints := []string{}
	if request.Constraints != nil && len(request.Constraints) > 0 {
		constraints = request.Constraints
	} else {
		constraints = linuxOnlyConstraints
	}

	spec.TaskTemplate.RestartPolicy.MaxAttempts = &maxRestarts
	spec.TaskTemplate.RestartPolicy.Condition = swarm.RestartPolicyConditionAny
	spec.TaskTemplate.RestartPolicy.Delay = &restartDelay
	spec.TaskTemplate.ContainerSpec.Image = request.Image

	labels, err := buildLabels(request)
	if err != nil {
		return err
	}

	spec.Annotations.Labels = labels
	spec.TaskTemplate.ContainerSpec.Labels = labels
	spec.TaskTemplate.ContainerSpec.Labels["com.openfaas.uid"] = fmt.Sprintf("%d", time.Now().Nanosecond())

	spec.TaskTemplate.Networks = []swarm.NetworkAttachmentConfig{
		{
			Target: network,
		},
	}

	spec.TaskTemplate.ContainerSpec.Secrets = secrets
	spec.TaskTemplate.ContainerSpec.ReadOnly = request.ReadOnlyRootFilesystem

	spec.TaskTemplate.ContainerSpec.Mounts = removeMounts(spec.TaskTemplate.ContainerSpec.Mounts, "/tmp")
	if request.ReadOnlyRootFilesystem {
		spec.TaskTemplate.ContainerSpec.Mounts = []mount.Mount{
			{
				Type:   mount.TypeTmpfs,
				Target: "/tmp",
			},
		}
	}

	spec.TaskTemplate.Resources = buildResources(request)

	spec.TaskTemplate.Placement = &swarm.Placement{
		Constraints: constraints,
	}

	spec.Annotations.Name = globalConfig.NFFaaSDockerProject + "_" + request.Service

	spec.RollbackConfig = &swarm.UpdateConfig{
		FailureAction: "pause",
	}

	spec.UpdateConfig = &swarm.UpdateConfig{
		Parallelism:   1,
		FailureAction: "rollback",
	}

	env := buildEnv(request.EnvProcess, request.EnvVars)

	if len(env) > 0 {
		spec.TaskTemplate.ContainerSpec.Env = env
	}

	if spec.Mode.Replicated != nil {
		spec.Mode.Replicated.Replicas = getMinReplicas(request)
	}

	return nil
}

// removeMounts returns a mount.Mount slice with any mounts matching target removed
// Uses the filter without allocation technique as described here
// https://github.com/golang/go/wiki/SliceTricks#filtering-without-allocating
func removeMounts(mounts []mount.Mount, target string) []mount.Mount {
	if mounts == nil {
		return nil
	}

	newMounts := mounts[:0]
	for _, v := range mounts {
		if v.Target != target {
			newMounts = append(newMounts, v)
		}
	}

	return newMounts
}
