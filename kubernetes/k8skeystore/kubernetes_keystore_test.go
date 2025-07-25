// Licensed to Elasticsearch B.V. under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. Elasticsearch B.V. licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package k8skeystore

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/elastic/elastic-agent-autodiscover/bus"
	"github.com/elastic/elastic-agent-libs/logp/logptest"
	"github.com/elastic/elastic-agent-libs/mapstr"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

const (
	ns         = "test_namespace"
	correctKey = "kubernetes.test_namespace.testing_secret.secret_value"
	pass       = "testing_passpass"
)

func TestGetKeystore(t *testing.T) {
	kRegistry := NewKubernetesKeystoresRegistry(nil, nil)
	k1 := kRegistry.GetKeystore(bus.Event{"kubernetes": mapstr.M{"namespace": "my_namespace"}})
	k2 := kRegistry.GetKeystore(bus.Event{"kubernetes": mapstr.M{"namespace": "my_namespace"}})
	assert.Equal(t, k1, k2)
	k3 := kRegistry.GetKeystore(bus.Event{"kubernetes": mapstr.M{"namespace": "my_namespace_2"}})
	assert.NotEqual(t, k2, k3)
}

func TestGetKeystoreAndRetrieve(t *testing.T) {
	client := k8sfake.NewSimpleClientset()
	secret := &v1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "apps/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testing_secret",
			Namespace: ns,
		},
		Data: map[string][]byte{
			"secret_value": []byte(pass),
		},
	}
	_, err := client.CoreV1().Secrets(ns).Create(context.Background(), secret, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create k8s secret: %v", err)
	}

	kRegistry := NewKubernetesKeystoresRegistry(nil, client)
	k1 := kRegistry.GetKeystore(bus.Event{"kubernetes": mapstr.M{"namespace": ns}})
	secure, err := k1.Retrieve(correctKey)
	if err != nil {
		t.Fatalf("could not retrieve k8s secret: %v", err)
	}
	secretVal, err := secure.Get()
	assert.NoError(t, err)
	bytePassword := []byte(pass)
	assert.Equal(t, bytePassword, secretVal)
}

func TestGetKeystoreAndRetrieveWithNonAllowedNamespace(t *testing.T) {
	kRegistry := getKeystoreForWrongValue(t)
	k1 := kRegistry.GetKeystore(bus.Event{"kubernetes": mapstr.M{"namespace": ns}})
	key := "kubernetes.test_namespace_HACK.testing_secret.secret_value"
	_, err := k1.Retrieve(key)
	assert.Error(t, err)
}

func TestGetKeystoreAndRetrieveWithWrongKeyFormat(t *testing.T) {
	kRegistry := getKeystoreForWrongValue(t)
	k1 := kRegistry.GetKeystore(bus.Event{"kubernetes": mapstr.M{"namespace": ns}})
	key := "HACK_test_namespace_HACK.testing_secret.secret_value"
	_, err := k1.Retrieve(key)
	assert.Error(t, err)
}

func TestGetKeystoreAndRetrieveWithNoSecretsExistent(t *testing.T) {
	logger := logptest.NewTestingLogger(t, "test_k8s_secrets")
	client := k8sfake.NewSimpleClientset()

	kRegistry := NewKubernetesKeystoresRegistry(logger, client)
	k1 := kRegistry.GetKeystore(bus.Event{"kubernetes": mapstr.M{"namespace": ns}})
	_, err := k1.Retrieve(correctKey)
	assert.Error(t, err)
}

func TestGetKeystoreAndRetrieveWithWrongSecretName(t *testing.T) {
	kRegistry := getKeystoreForWrongValue(t)
	k1 := kRegistry.GetKeystore(bus.Event{"kubernetes": mapstr.M{"namespace": ns}})
	key := "kubernetes.test_namespace.testing_secret_WRONG.secret_value"
	_, err := k1.Retrieve(key)
	assert.Error(t, err)
}

func TestGetKeystoreAndRetrieveWithWrongSecretValue(t *testing.T) {
	kRegistry := getKeystoreForWrongValue(t)
	k1 := kRegistry.GetKeystore(bus.Event{"kubernetes": mapstr.M{"namespace": ns}})
	key := "kubernetes.test_namespace.testing_secret.secret_value_WRONG"
	_, err := k1.Retrieve(key)
	assert.Error(t, err)
}

func getKeystoreForWrongValue(t *testing.T) bus.KeystoreProvider {
	logger := logptest.NewTestingLogger(t, "test_k8s_secrets")
	client := k8sfake.NewSimpleClientset()
	secret := &v1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "apps/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testing_secret",
			Namespace: ns,
		},
		Data: map[string][]byte{
			"secret_value": []byte(pass),
		},
	}
	_, err := client.CoreV1().Secrets(ns).Create(context.Background(), secret, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create k8s secret: %v", err)
	}

	return NewKubernetesKeystoresRegistry(logger, client)
}
