// Copyright 2017 Microsoft. All rights reserved.
// MIT License

//go:build windows
// +build windows

package hnswrapper

import (
	"github.com/Microsoft/hcsshim/hcn"
)

type Hnsv2wrapperFake struct {
}

func (f Hnsv2wrapperFake) CreateNetwork(network *hcn.HostComputeNetwork) (*hcn.HostComputeNetwork, error) {
	return network, nil
}

func (f Hnsv2wrapperFake) DeleteNetwork(network *hcn.HostComputeNetwork) error {
	return nil
}

func (Hnsv2wrapperFake) ModifyNetworkSettings(network *hcn.HostComputeNetwork, request *hcn.ModifyNetworkSettingRequest) error {
	return nil
}

func (Hnsv2wrapperFake) AddNetworkPolicy(network *hcn.HostComputeNetwork, networkPolicy hcn.PolicyNetworkRequest) error {
	return nil
}

func (Hnsv2wrapperFake) RemoveNetworkPolicy(network *hcn.HostComputeNetwork, networkPolicy hcn.PolicyNetworkRequest) error {
	return nil
}

func (Hnsv2wrapperFake) GetNetworkByName(networkName string) (*hcn.HostComputeNetwork, error) {
	return &hcn.HostComputeNetwork{}, nil
}

func (f Hnsv2wrapperFake) GetNetworkByID(networkID string) (*hcn.HostComputeNetwork, error) {
	network := &hcn.HostComputeNetwork{Id: networkID}
	return network, nil
}

func (f Hnsv2wrapperFake) GetEndpointByID(endpointID string) (*hcn.HostComputeEndpoint, error) {
	endpoint := &hcn.HostComputeEndpoint{Id: endpointID}
	return endpoint, nil
}

func (Hnsv2wrapperFake) CreateEndpoint(endpoint *hcn.HostComputeEndpoint) (*hcn.HostComputeEndpoint, error) {
	return endpoint, nil
}

func (Hnsv2wrapperFake) DeleteEndpoint(endpoint *hcn.HostComputeEndpoint) error {
	return nil
}

func (Hnsv2wrapperFake) GetNamespaceByID(netNamespacePath string) (*hcn.HostComputeNamespace, error) {
	nameSpace := &hcn.HostComputeNamespace{Id: "ea37ac15-119e-477b-863b-cc23d6eeaa4d", NamespaceId: 1000}
	return nameSpace, nil
}

func (Hnsv2wrapperFake) AddNamespaceEndpoint(namespaceId string, endpointId string) error {
	return nil
}

func (Hnsv2wrapperFake) RemoveNamespaceEndpoint(namespaceId string, endpointId string) error {
	return nil
}

func (Hnsv2wrapperFake) ListEndpointsOfNetwork(networkId string) ([]hcn.HostComputeEndpoint, error) {
	return []hcn.HostComputeEndpoint{}, nil
}

func (Hnsv2wrapperFake) ApplyEndpointPolicy(endpoint *hcn.HostComputeEndpoint, requestType hcn.RequestType, endpointPolicy hcn.PolicyEndpointRequest) error {
	return nil
}
