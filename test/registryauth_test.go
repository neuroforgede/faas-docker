// Copyright (c) Alex Ellis 2017. All rights reserved.
// Licensed under the MIT license. See LICENSE file in the project root for full license information.

package test

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/neuroforgede/nf-faas-docker/handlers"
)

func TestBuildEncodedAuthConfig(t *testing.T) {
	// custom repository with valid data
	testValidEncodedAuthConfig(t, "user", "password", "my.repository.com/user/imagename", "my.repository.com")
	testValidEncodedAuthConfig(t, "user", "password", "my.repository.com/user/imagename:v0.1", "my.repository.com")
	testValidEncodedAuthConfig(t, "user", "password", "my.repository.com/user/imagename:latest", "my.repository.com")
	testValidEncodedAuthConfig(t, "user", "weird:password:", "my.repository.com/user/imagename", "my.repository.com")
	testValidEncodedAuthConfig(t, "userWithNoPassword", "", "my.repository.com/user/imagename", "my.repository.com")
	testValidEncodedAuthConfig(t, "", "", "my.repository.com/user/imagename", "my.repository.com")

	// docker hub default repository
	testValidEncodedAuthConfig(t, "user", "password", "user/imagename", "docker.io")
	testValidEncodedAuthConfig(t, "user", "password", "user/imagename:v0.1", "docker.io")
	testValidEncodedAuthConfig(t, "user", "password", "user/imagename:latest", "docker.io")
	testValidEncodedAuthConfig(t, "user", "password", "docker.io/user/imagename", "docker.io")
	testValidEncodedAuthConfig(t, "user", "password", "docker.io/user/imagename:v0.1", "docker.io")
	testValidEncodedAuthConfig(t, "user", "password", "docker.io/user/imagename:latest", "docker.io")
	testValidEncodedAuthConfig(t, "", "", "docker.io/user/imagename", "docker.io")

	// invalid base64 basic auth
	assertEncodedAuthError(t, "invalidBasicAuth", "my.repository.com/user/imagename")

	// invalid docker image name
	assertEncodedAuthError(t, b64BasicAuth("user", "password"), "")
	assertEncodedAuthError(t, b64BasicAuth("user", "password"), "invalid name")
}

func testValidEncodedAuthConfig(t *testing.T, user, password, imageName, expectedRegistryHost string) {
	encodedAuthConfig, err := handlers.BuildEncodedAuthConfig(b64BasicAuth(user, password), imageName)
	if err != nil {
		t.Log("Unexpected error while building auth config with correct values")
		t.Fail()
	}

	authConfig := &types.AuthConfig{}
	authJSON := base64.NewDecoder(base64.URLEncoding, strings.NewReader(encodedAuthConfig))
	if err := json.NewDecoder(authJSON).Decode(authConfig); err != nil {
		t.Log("Invalid encoded auth", err)
		t.Fail()
	}

	if user != authConfig.Username {
		t.Logf("Auth config username mismatch want %s, got: %s", user, authConfig.Username)
		t.Fail()
	}

	if password != authConfig.Password {
		t.Logf("Auth config password mismatch want: %s, got: %s", password, authConfig.Password)
		t.Fail()
	}

	if expectedRegistryHost != authConfig.ServerAddress {

		t.Logf("Auth config registry server address mismatch want: %s, got: %s", expectedRegistryHost, authConfig.ServerAddress)
		t.Fail()
	}
}

func assertEncodedAuthError(t *testing.T, b64BasicAuth, imageName string) {
	_, err := handlers.BuildEncodedAuthConfig(b64BasicAuth, imageName)
	if err == nil {
		t.Log("Expected an error to be returned")
		t.Fail()
	}
}

func b64BasicAuth(user, password string) string {
	return base64.StdEncoding.EncodeToString([]byte(user + ":" + password))
}
