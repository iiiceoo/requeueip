/*
Copyright 2024 The RequeueIP Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	requeueipv1 "github.com/iiiceoo/requeueip/api/v1"
)

const (
	busybox      = "busybox"
	busyboxImage = busybox
)

var scheme *runtime.Scheme
var c client.Client
var e2eLabels map[string]string

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2E Suite", Label("e2e"))
}

var _ = BeforeSuite(func(ctx SpecContext) {
	scheme = runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())
	err = requeueipv1.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	conf, err := config.GetConfig()
	Expect(err).NotTo(HaveOccurred())
	Expect(conf).NotTo(BeNil())

	conf.Burst = 150
	conf.QPS = 100
	c, err = client.New(conf, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(c).NotTo(BeNil())
})
