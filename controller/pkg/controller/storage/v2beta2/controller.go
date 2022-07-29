// SPDX-FileCopyrightText: 2022-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package v2beta2

import (
	"github.com/atomix/runtime/sdk/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var log = logging.GetLogger()

// AddControllers creates a new Partition ManagementGroup and adds it to the Manager. The Manager will set fields on the ManagementGroup
// and Start it when the Manager is Started.
func AddControllers(mgr manager.Manager) error {
	if err := addMultiRaftStoreController(mgr); err != nil {
		return err
	}
	if err := addPodController(mgr); err != nil {
		return err
	}
	return nil
}

func hasFinalizer(object client.Object, name string) bool {
	for _, finalizer := range object.GetFinalizers() {
		if finalizer == name {
			return true
		}
	}
	return false
}

func addFinalizer(object client.Object, name string) {
	object.SetFinalizers(append(object.GetFinalizers(), name))
}

func removeFinalizer(object client.Object, name string) {
	var finalizers []string
	for _, finalizer := range object.GetFinalizers() {
		if finalizer != name {
			finalizers = append(finalizers, finalizer)
		}
	}
	object.SetFinalizers(finalizers)
}