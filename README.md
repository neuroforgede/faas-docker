Fork maintained by [NeuroForge](https://neuroforge.de/) in Bayreuth, Germany.

nf-faas-docker
==============

An unofficial provider for Docker Swarm for OpenFaaS Community Edition (CE)

This is based on the original Docker Swarm provider, but not a drop in replacement. It is actually incompatible with existing installations.
Supports faas-provider SDK v0.19.1

Deployment code is at https://github.com/neuroforgede/nf-faas-docker-stack

## Summary

This project adds support for Docker Swarm for usage in modern versions of OpenFaaS ®.

We do not aim to keep backwards support for existing deployments using faas-swarm. If you need help migrating, please reach out in the discussions.
## Status

Status: Released

Features:

* [x] Create
* [x] Proxy
* [x] Update
* [x] Delete
* [x] List
* [x] Scale

Not Supported:

- [ ] Namespaces - Docker Swarm does not have Namespaces. Instead deploy the OpenFaaS stack multiple times with different projects.

Additional Changes for sharing a single Swarm between multiple OpenFaaS instances:

- [x] Allow for multiple providers to run in the same swarm. Specified via `NF_FAAS_DOCKER_PROJECT` env var
- [x] prefix function name services with project name
- [x] prefix secret names with project name

Docker image: [`neuroforgede/nf-faas-docker`](https://hub.docker.com/r/neuroforgede/nf-faas-docker/tags/)

## Trademark notice

OpenFaaS® is a trademark of OpenFaaS Ltd. OpenFaaS ® is a registered trademark in England and Wales.

OpenFaaS Ltd. is not associated with this project. This projects is based off of MIT licensed code from https://github.com/openfaas/faas-swarm.
