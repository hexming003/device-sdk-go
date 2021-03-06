//
// Copyright (C) 2018 IOTech Ltd
//
// SPDX-License-Identifier: Apache-2.0

package device

import (
	"fmt"
	"github.com/edgexfoundry/device-sdk-go/common"
	"github.com/edgexfoundry/device-sdk-go/config"
	"github.com/edgexfoundry/device-sdk-go/registry"
	"github.com/edgexfoundry/edgex-go/pkg/clients/coredata"
	"github.com/edgexfoundry/edgex-go/pkg/clients/logging"
	"github.com/edgexfoundry/edgex-go/pkg/clients/metadata"
	"github.com/edgexfoundry/edgex-go/pkg/clients/types"
	consulapi "github.com/hashicorp/consul/api"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// initDependencyClients
// Trigger Service Client Initializer to establish connection to Metadata and Core Data Services through Metadata Client and Core Data Client.
// Service Client Initializer also needs to check the service status of Metadata and Core Data Services, because they are important dependencies of Device Service.
// The initialization process should be pending until Metadata Service and Core Data Service are both available.
func initDependencyClients() {
	initializeLoggingClient()

	checkDependencyServices()

	initializeClients()

	checkClientsInitialSuccessful()

	svc.lc.Info("Service clients initialize successful.")
}

func initializeLoggingClient() {
	var remoteLog = false
	var logTarget string

	if svc.config.Logging.RemoteURL == "" {
		logTarget = svc.config.Logging.File

	} else if checkRemoteLoggingAvailable() {
		remoteLog = true
		logTarget = svc.config.Logging.RemoteURL
		fmt.Println("Ping remote logging service success, use remote logging.")
	} else {
		logTarget = svc.config.Logging.File
		fmt.Println("Ping remote logging service failed, use log file instead.")
	}

	svc.lc = logger.NewClient(svc.Name, remoteLog, logTarget)
}

func checkRemoteLoggingAvailable() bool {
	var available = true
	fmt.Println("Check Logging service's status ...")

	_, err := http.Get(svc.config.Logging.RemoteURL + common.APIPrefix + "/ping")
	if err != nil {
		fmt.Println(fmt.Sprintf("Error getting ping: %v", err))
		available = false
	}

	return available
}

func checkDependencyServices() {
	var dependencyList = []string{common.ClientData, common.ClientMetadata}

	var waitGroup sync.WaitGroup
	waitGroup.Add(len(dependencyList))

	for i := 0; i < len(dependencyList); i++ {
		go func(wg *sync.WaitGroup, serviceName string) {
			checkServiceAvailable(serviceName)
			wg.Done()

		}(&waitGroup, dependencyList[i])

	}

	waitGroup.Wait()
}

func checkServiceAvailable(serviceId string) {
	if svc.useRegistry {
		if !checkServiceAvailableByConsul(svc.config.Clients[serviceId].Name) {
			time.Sleep(10 * time.Second)
			checkServiceAvailable(serviceId)
		}
	} else {
		var err = checkServiceAvailableByPing(serviceId)
		if err, ok := err.(net.Error); ok && err.Timeout() {
			checkServiceAvailable(serviceId)
		} else if err != nil {
			time.Sleep(10 * time.Second)
			checkServiceAvailable(serviceId)
		}
	}
}

func checkServiceAvailableByPing(serviceId string) error {
	svc.lc.Info(fmt.Sprintf("Check %v service's status ...", serviceId))
	host := svc.config.Clients[serviceId].Host
	port := strconv.Itoa(svc.config.Clients[serviceId].Port)
	addr := common.BuildAddr(host, port)
	timeout := int64(svc.config.Clients[serviceId].Timeout) * int64(time.Millisecond)

	client := http.Client{
		Timeout: time.Duration(timeout),
	}

	_, err := client.Get(addr + common.APIPrefix + "/ping")

	if err != nil {
		svc.lc.Error(fmt.Sprintf("Error getting ping: %v ", err))
	}
	return err
}

func checkServiceAvailableByConsul(serviceConsulId string) bool {
	svc.lc.Info(fmt.Sprintf("Check %v service's status by Consul...", serviceConsulId))

	result := false

	isConsulUp := checkConsulAvailable()
	if !isConsulUp {
		return false
	}

	// Get a new client
	var host = svc.config.Registry.Host
	var port = strconv.Itoa(svc.config.Registry.Port)
	var consulAddr = common.BuildAddr(host, port)
	consulConfig := consulapi.DefaultConfig()
	consulConfig.Address = consulAddr
	client, err := consulapi.NewClient(consulConfig)
	if err != nil {
		svc.lc.Error(err.Error())
		return false
	}

	services, _, err := client.Catalog().Service(serviceConsulId, "", nil)
	if err != nil {
		svc.lc.Error(err.Error())
		return false
	}
	if len(services) <= 0 {
		svc.lc.Error(serviceConsulId + " service hasn't started...")
		return false
	}

	healthCheck, _, err := client.Health().Checks(serviceConsulId, nil)
	if err != nil {
		svc.lc.Error(err.Error())
		return false
	}
	status := healthCheck.AggregatedStatus()
	if status == "passing" {
		result = true
	} else {
		svc.lc.Error(serviceConsulId + " service hasn't been available...")
		result = false
	}

	return result
}

func checkConsulAvailable() bool {
	addr := fmt.Sprintf("%v:%v", svc.config.Registry.Host, svc.config.Registry.Port)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		svc.lc.Error(fmt.Sprintf("Consul cannot be reached, address: %v and error is \"%v\" ", addr, err.Error()))
		return false
	}
	conn.Close()
	return true
}

func initializeClients() {
	consulEndpoint := &registry.ConsulEndpoint{RegistryClient: config.RegistryClient}

	metaPort := strconv.Itoa(svc.config.Clients[common.ClientMetadata].Port)
	metaHost := svc.config.Clients[common.ClientMetadata].Host
	metaAddr := common.BuildAddr(metaHost, metaPort)

	dataPort := strconv.Itoa(svc.config.Clients[common.ClientData].Port)
	dataHost := svc.config.Clients[common.ClientData].Host
	dataAddr := common.BuildAddr(dataHost, dataPort)

	params := types.EndpointParams{
		UseRegistry: svc.useRegistry,
	}

	// initialize Core Metadata clients
	params.ServiceKey = svc.config.Clients[common.ClientMetadata].Name

	params.Path = common.V1Addressable
	params.Url = metaAddr + params.Path
	svc.ac = metadata.NewAddressableClient(params, consulEndpoint)

	params.Path = common.V1Device
	params.Url = metaAddr + params.Path
	svc.dc = metadata.NewDeviceClient(params, consulEndpoint)

	params.Path = common.V1DevService
	params.Url = metaAddr + params.Path
	svc.sc = metadata.NewDeviceServiceClient(params, consulEndpoint)

	params.Path = v1Deviceprofile
	params.Url = metaAddr + params.Path
	svc.dpc = metadata.NewDeviceProfileClient(params, consulEndpoint)

	params.Path = v1Schedule
	params.Url = metaAddr + params.Path
	svc.scc = metadata.NewScheduleClient(params, consulEndpoint)

	params.Path = v1ScheduleEvent
	params.Url = metaAddr + params.Path
	svc.scec = metadata.NewScheduleEventClient(params, consulEndpoint)

	// initialize Core Data clients
	params.ServiceKey = svc.config.Clients[common.ClientData].Name

	params.Path = common.V1Event
	params.Url = dataAddr + params.Path
	svc.ec = coredata.NewEventClient(params, consulEndpoint)

	params.Path = v1Valuedescriptor
	params.Url = dataAddr + params.Path
	svc.vdc = coredata.NewValueDescriptorClient(params, consulEndpoint)

}

// checkClientsInitialSuccessful is used to check some clients need operate immediately.
// So far we only add default schedule and scheduleEvent after client initialize.
func checkClientsInitialSuccessful() {
	_, err := svc.scc.Schedules()
	if err != nil {
		svc.lc.Warn(fmt.Sprintf("Metadata.schedule client has not been initialized yet... Error: %v . Wait a seconds.", err.Error()))
		time.Sleep(2 * time.Second)
		checkClientsInitialSuccessful()
	}

	_, err = svc.scec.ScheduleEvents()
	if err != nil {
		svc.lc.Warn(fmt.Sprintf("Metadata.scheduleEvent has not been initialized yet... Error: %v . Wait a seconds.", err.Error()))
		time.Sleep(2 * time.Second)
		checkClientsInitialSuccessful()
	}
}
