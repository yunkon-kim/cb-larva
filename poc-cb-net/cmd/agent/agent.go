package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	cbnet "github.com/cloud-barista/cb-larva/poc-cb-net/internal/cb-network"
	model "github.com/cloud-barista/cb-larva/poc-cb-net/internal/cb-network/model"
	cmd "github.com/cloud-barista/cb-larva/poc-cb-net/internal/command"
	etcdkey "github.com/cloud-barista/cb-larva/poc-cb-net/internal/etcd-key"
	"github.com/cloud-barista/cb-larva/poc-cb-net/internal/file"
	cblog "github.com/cloud-barista/cb-log"
	"github.com/go-ping/ping"
	"github.com/sirupsen/logrus"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// CBNet represents a network for the multi-cloud.
var CBNet *cbnet.CBNetwork

// CBLogger represents a logger to show execution processes according to the logging level.
var CBLogger *logrus.Logger
var config model.Config

func init() {
	fmt.Println("Start......... init() of agent.go")
	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	exePath := filepath.Dir(ex)
	fmt.Printf("exePath: %v\n", exePath)

	// Load cb-log config from the current directory (usually for the production)
	logConfPath := filepath.Join(exePath, "config", "log_conf.yaml")
	fmt.Printf("logConfPath: %v\n", logConfPath)
	if !file.Exists(logConfPath) {
		// Load cb-log config from the project directory (usually for development)
		path, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
		if err != nil {
			panic(err)
		}
		projectPath := strings.TrimSpace(string(path))
		logConfPath = filepath.Join(projectPath, "poc-cb-net", "config", "log_conf.yaml")
	}
	CBLogger = cblog.GetLoggerWithConfigPath("cb-network", logConfPath)
	CBLogger.Debugf("Load %v", logConfPath)

	// Load cb-network config from the current directory (usually for the production)
	configPath := filepath.Join(exePath, "config", "config.yaml")
	fmt.Printf("configPath: %v\n", configPath)
	if !file.Exists(configPath) {
		// Load cb-network config from the project directory (usually for the development)
		path, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
		if err != nil {
			panic(err)
		}
		projectPath := strings.TrimSpace(string(path))
		configPath = filepath.Join(projectPath, "poc-cb-net", "config", "config.yaml")
	}
	config, _ = model.LoadConfig(configPath)
	CBLogger.Debugf("Load %v", configPath)
	fmt.Println("End......... init() of agent.go")
}

// Control the cb-network agent by commands from remote
func watchControlCommand(etcdClient *clientv3.Client, wg *sync.WaitGroup) {
	CBLogger.Debug("Start.........")

	defer wg.Done()

	cladnetID := CBNet.ID
	hostID := CBNet.HostID

	// Watch "/registry/cloud-adaptive-network/control-command/{cladnet-id}/{host-id}
	keyControlCommand := fmt.Sprint(etcdkey.ControlCommand + "/" + cladnetID + "/" + hostID)
	CBLogger.Tracef("Watch \"%v\"", keyControlCommand)
	watchChan1 := etcdClient.Watch(context.TODO(), keyControlCommand)
	for watchResponse := range watchChan1 {
		for _, event := range watchResponse.Events {
			CBLogger.Tracef("Watch - %s %q : %q", event.Type, event.Kv.Key, event.Kv.Value)

			controlCommand, controlCommandOption := cmd.ParseCommandMessage(string(event.Kv.Value))

			handleCommand(controlCommand, controlCommandOption, etcdClient)
		}
	}
	CBLogger.Debug("Start.........")
}

// Handle commands of the cb-network agent
func handleCommand(command string, commandOption string, etcdClient *clientv3.Client) {
	CBLogger.Debug("Start.........")

	CBLogger.Debugf("Command: %+v", command)
	CBLogger.Tracef("CommandOption: %+v", commandOption)
	switch command {
	case "suspend":
		updatePeerState(model.Suspended, etcdClient)
		CBNet.Shutdown()

	case "resume":

		// Start the cb-network
		go CBNet.Startup()

		// Watch the networking rule to update dynamically
		go watchNetworkingRule(etcdClient)

		// Watch the other agents' secrets (RSA public keys)
		if CBNet.IsEncryptionEnabled() {
			go watchSecret(etcdClient)
		}

		// Sleep until the all routines are ready
		time.Sleep(2 * time.Second)

		// Try Compare-And-Swap (CAS) an agent's secret (RSA public keys)
		if CBNet.IsEncryptionEnabled() {
			compareAndSwapSecret(etcdClient)
		}
		//time.Sleep(2 * time.Second)

		// Try Compare-And-Swap (CAS) a host-network-information by cladnetID and hostId
		compareAndSwapHostNetworkInformation(etcdClient)

	case "check-connectivity":
		checkConnectivity(commandOption, etcdClient)

	default:
		CBLogger.Trace("Default ?")
	}

	CBLogger.Debug("End.........")
}

func watchNetworkingRule(etcdClient *clientv3.Client) {
	CBLogger.Debug("Start.........")

	// Watch "/registry/cloud-adaptive-network/networking-rule/{cladnet-id}" with version
	keyNetworkingRule := fmt.Sprint(etcdkey.NetworkingRule + "/" + CBNet.ID)
	CBLogger.Tracef("Watch \"%v\"", keyNetworkingRule)
	watchChan1 := etcdClient.Watch(context.TODO(), keyNetworkingRule, clientv3.WithPrefix())
	for watchResponse := range watchChan1 {
		for _, event := range watchResponse.Events {
			CBLogger.Tracef("Watch - %s %q : %q", event.Type, event.Kv.Key, event.Kv.Value)

			key := string(event.Kv.Key)
			peerBytes := event.Kv.Value

			// Update a host's configuration in the networking rule
			isThisPeerInitialized := CBNet.UpdatePeer(peerBytes)

			if isThisPeerInitialized {
				var peer model.Peer
				if err := json.Unmarshal(peerBytes, &peer); err != nil {
					CBLogger.Error(err)
				}
				peer.State = model.Running

				CBLogger.Debugf("Put \"%v\"", key)
				doc, _ := json.Marshal(peer)

				if _, err := etcdClient.Put(context.TODO(), key, string(doc)); err != nil {
					CBLogger.Error(err)
				}
			}
		}
	}
	CBLogger.Debug("End.........")
}

func compareAndSwapHostNetworkInformation(etcdClient *clientv3.Client) {
	CBLogger.Debug("Start.........")

	cladnetID := CBNet.ID
	hostID := CBNet.HostID

	CBLogger.Debug("Get the host network information")
	CBNet.UpdateHostNetworkInformation()
	temp := CBNet.GetHostNetworkInformation()
	currentHostNetworkInformationBytes, _ := json.Marshal(temp)
	currentHostNetworkInformation := string(currentHostNetworkInformationBytes)
	CBLogger.Trace(currentHostNetworkInformation)

	CBLogger.Debug("Compare-And-Swap (CAS) the host network information")
	keyNetworkingRule := fmt.Sprint(etcdkey.NetworkingRule + "/" + cladnetID)
	keyHostNetworkInformation := fmt.Sprint(etcdkey.HostNetworkInformation + "/" + cladnetID + "/" + hostID)
	// NOTICE: "!=" doesn't work..... It might be a temporal issue.
	txnResp, err := etcdClient.Txn(context.Background()).
		If(clientv3.Compare(clientv3.Value(keyHostNetworkInformation), "=", currentHostNetworkInformation)).
		Then(clientv3.OpGet(keyNetworkingRule, clientv3.WithPrefix())).
		Else(clientv3.OpPut(keyHostNetworkInformation, currentHostNetworkInformation)).
		Commit()

	if err != nil {
		CBLogger.Error(err)
	}
	CBLogger.Tracef("Transaction Response: %v", txnResp)

	// The CAS would be succeeded if the prev host network information and current host network information are same.
	// Then the networking rule will be returned. (The above "watch" will not be performed.)
	// If not, the host tries to put the current host network information.
	if txnResp.Succeeded {
		// Set the networking rule to the host
		for _, kv := range txnResp.Responses[0].GetResponseRange().Kvs {
			key := string(kv.Key)
			CBLogger.Tracef("Key: %v", key)

			peerBytes := kv.Value
			CBLogger.Tracef("A host's configuration: %v", peerBytes)
			CBLogger.Debug("Update a host's configuration")

			// Update a host's configuration in the networking rule
			isThisPeerInitialized := CBNet.UpdatePeer(peerBytes)

			if isThisPeerInitialized {
				var peer model.Peer
				if err := json.Unmarshal(peerBytes, &peer); err != nil {
					CBLogger.Error(err)
				}
				peer.State = model.Running

				CBLogger.Debugf("Put \"%v\"", key)
				doc, _ := json.Marshal(peer)

				if _, err := etcdClient.Put(context.TODO(), key, string(doc)); err != nil {
					CBLogger.Error(err)
				}
			}
		}
	}

	CBLogger.Debug("End.........")
}

func updatePeerState(state string, etcdClient *clientv3.Client) {

	idx := CBNet.NetworkingRule.GetIndexOfHostID(CBNet.HostID)

	peer := &model.Peer{
		CLADNetID:          CBNet.NetworkingRule.CLADNetID,
		HostID:             CBNet.NetworkingRule.HostID[idx],
		PrivateIPv4Network: CBNet.NetworkingRule.HostIPv4Network[idx],
		PrivateIPv4Address: CBNet.NetworkingRule.HostIPAddress[idx],
		PublicIPv4Address:  CBNet.NetworkingRule.PublicIPAddress[idx],
		State:              state,
	}

	keyNetworkingRuleOfPeer := fmt.Sprint(etcdkey.NetworkingRule + "/" + CBNet.ID + "/" + CBNet.HostID)

	CBLogger.Debugf("Put \"%v\"", keyNetworkingRuleOfPeer)
	doc, _ := json.Marshal(peer)

	if _, err := etcdClient.Put(context.TODO(), keyNetworkingRuleOfPeer, string(doc)); err != nil {
		CBLogger.Error(err)
	}
}

func watchSecret(etcdClient *clientv3.Client) {
	CBLogger.Debug("Start.........")

	// Watch "/registry/cloud-adaptive-network/secret/{cladnet-id}"
	keySecretGroup := fmt.Sprint(etcdkey.Secret + "/" + CBNet.ID)
	CBLogger.Tracef("Watch \"%v\"", keySecretGroup)
	watchChan1 := etcdClient.Watch(context.TODO(), keySecretGroup, clientv3.WithPrefix())
	for watchResponse := range watchChan1 {
		for _, event := range watchResponse.Events {
			CBLogger.Tracef("Watch - %s %q : %q", event.Type, event.Kv.Key, event.Kv.Value)
			slicedKeys := strings.Split(string(event.Kv.Key), "/")
			parsedHostID := slicedKeys[len(slicedKeys)-1]
			CBLogger.Tracef("ParsedHostID: %v", parsedHostID)

			// Update keyring (including add)
			CBNet.UpdateKeyring(parsedHostID, string(event.Kv.Value))
		}
	}
	CBLogger.Debug("End.........")
}

func compareAndSwapSecret(etcdClient *clientv3.Client) {
	CBLogger.Debug("Start.........")

	CBLogger.Debug("Compare-And-Swap (CAS) an agent's secret")
	// Watch "/registry/cloud-adaptive-network/secret/{cladnet-id}"
	KeySecretGroup := fmt.Sprint(etcdkey.Secret + "/" + CBNet.ID)
	keySecretHost := fmt.Sprint(etcdkey.Secret + "/" + CBNet.ID + "/" + CBNet.HostID)

	base64PublicKey, _ := CBNet.GetPublicKeyBase64()
	CBLogger.Tracef("Base64PublicKey: %+v", base64PublicKey)

	// NOTICE: "!=" doesn't work..... It might be a temporal issue.
	txnResp, err := etcdClient.Txn(context.Background()).
		If(clientv3.Compare(clientv3.Value(keySecretHost), "=", base64PublicKey)).
		Then(clientv3.OpGet(KeySecretGroup, clientv3.WithPrefix())).
		Else(clientv3.OpPut(keySecretHost, base64PublicKey)).
		Commit()

	if err != nil {
		CBLogger.Error(err)
	}
	CBLogger.Tracef("Transaction Response: %#v", txnResp)

	// The CAS would be succeeded if the prev host network information and current host network information are same.
	// Then the networking rule will be returned. (The above "watch" will not be performed.)
	// If not, the host tries to put the current host network information.

	if txnResp.Succeeded {
		// Set the networking rule to the host
		for _, kv := range txnResp.Responses[0].GetResponseRange().Kvs {
			respKey := kv.Key
			slicedKeys := strings.Split(string(respKey), "/")
			parsedHostID := slicedKeys[len(slicedKeys)-1]
			CBLogger.Tracef("ParsedHostID: %v", parsedHostID)

			// Update keyring (including add)
			CBNet.UpdateKeyring(parsedHostID, string(kv.Value))
		}
	}

	CBLogger.Debug("End.........")
}

func checkConnectivity(data string, etcdClient *clientv3.Client) {
	CBLogger.Debug("Start.........")

	cladnetID := CBNet.ID
	hostID := CBNet.HostID

	// Get the trial count
	var testSpecification model.TestSpecification
	errUnmarshalEvalSpec := json.Unmarshal([]byte(data), &testSpecification)
	if errUnmarshalEvalSpec != nil {
		CBLogger.Error(errUnmarshalEvalSpec)
	}
	trialCount := testSpecification.TrialCount

	// Check status of a CLADNet
	networkingRule := CBNet.NetworkingRule
	idx := networkingRule.GetIndexOfPublicIP(CBNet.HostPublicIP)
	sourceID := networkingRule.HostID[idx]
	sourceIP := networkingRule.HostIPAddress[idx]

	// Perform ping test from this host to another host
	listLen := len(networkingRule.HostIPAddress)
	outSize := listLen - 1 // -1: except this host
	var testwg sync.WaitGroup
	out := make([]model.InterHostNetworkStatus, outSize)

	j := 0
	for i := 0; i < listLen; i++ {

		if idx == i { // if source == destination
			continue
		}

		out[j].SourceID = sourceID
		out[j].SourceIP = sourceIP
		out[j].DestinationID = networkingRule.HostID[i]
		out[j].DestinationIP = networkingRule.HostIPAddress[i]

		testwg.Add(1)
		go pingTest(&out[j], &testwg, trialCount)
		j++
	}
	testwg.Wait()

	// Gather the evaluation results
	var networkStatus model.NetworkStatus
	for i := 0; i < len(out); i++ {
		networkStatus.InterHostNetworkStatus = append(networkStatus.InterHostNetworkStatus, out[i])
	}

	if networkStatus.InterHostNetworkStatus == nil {
		networkStatus.InterHostNetworkStatus = make([]model.InterHostNetworkStatus, 0)
	}

	// Put the network status of the CLADNet to the etcd
	// Key: /registry/cloud-adaptive-network/status/information/{cladnet-id}/{host-id}
	keyStatusInformation := fmt.Sprint(etcdkey.StatusInformation + "/" + cladnetID + "/" + hostID)
	strNetworkStatus, _ := json.Marshal(networkStatus)
	_, err := etcdClient.Put(context.Background(), keyStatusInformation, string(strNetworkStatus))
	if err != nil {
		CBLogger.Error(err)
	}

	CBLogger.Debug("End.........")
}

func pingTest(outVal *model.InterHostNetworkStatus, wg *sync.WaitGroup, trialCount int) {
	CBLogger.Debug("Start.........")
	defer wg.Done()

	CBLogger.Tracef("Ping to %s", outVal.DestinationIP)

	pinger, err := ping.NewPinger(outVal.DestinationIP)
	pinger.SetPrivileged(true)
	if err != nil {
		CBLogger.Error(err)
	}
	//pinger.OnRecv = func(pkt *ping.Packet) {
	//	fmt.Printf("%d bytes from %s: icmp_seq=%d time=%v\n",
	//		pkt.Nbytes, pkt.IPAddr, pkt.Seq, pkt.Rtt)
	//}

	pinger.Count = trialCount
	pinger.Run() // blocks until finished

	stats := pinger.Statistics() // get send/receive/rtt stats
	outVal.MininumRTT = stats.MinRtt.Seconds()
	outVal.AverageRTT = stats.AvgRtt.Seconds()
	outVal.MaximumRTT = stats.MaxRtt.Seconds()
	outVal.StdDevRTT = stats.StdDevRtt.Seconds()
	outVal.PacketsReceive = stats.PacketsRecv
	outVal.PacketsLoss = stats.PacketsSent - stats.PacketsRecv
	outVal.BytesReceived = stats.PacketsRecv * 24

	CBLogger.Tracef("round-trip min/avg/max/stddev/dupl_recv = %v/%v/%v/%v/%v bytes",
		stats.MinRtt, stats.AvgRtt, stats.MaxRtt, stats.StdDevRtt, stats.PacketsRecv*24)
	CBLogger.Debug("End.........")
}

func main() {
	CBLogger.Debug("Start.........")

	// etcd Section
	// Connect to the etcd cluster
	etcdClient, err := clientv3.New(clientv3.Config{
		Endpoints:   config.ETCD.Endpoints,
		DialTimeout: 5 * time.Second,
	})

	if err != nil {
		CBLogger.Fatal(err)
	}

	defer func() {
		errClose := etcdClient.Close()
		if errClose != nil {
			CBLogger.Fatal("Can't close the etcd client", errClose)
		}
	}()

	CBLogger.Infoln("The etcdClient is connected.")

	// Cloud Adaptive Network section
	// Config
	var cladnetID = config.CBNetwork.CLADNetID
	var hostID string
	if config.CBNetwork.HostID == "" {
		name, err := os.Hostname()
		if err != nil {
			CBLogger.Error(err)
		}
		hostID = name
	} else {
		hostID = config.CBNetwork.HostID
	}

	// Create CBNetwork instance with port, which is a tunneling port
	CBNet = cbnet.New("cbnet0", 20000)
	CBNet.ID = cladnetID
	CBNet.HostID = hostID

	// Enable encryption or not
	CBNet.EnableEncryption(config.CBNetwork.IsEncrypted)

	// Wait for multiple goroutines to complete
	var wg sync.WaitGroup

	wg.Add(1)
	go watchControlCommand(etcdClient, &wg)

	// Sleep until the all routines are ready
	time.Sleep(2 * time.Second)

	// Resume cb-network
	handleCommand("resume", "", etcdClient)

	// Waiting for all goroutines to finish
	CBLogger.Info("Waiting for all goroutines to finish")
	wg.Wait()

	CBLogger.Debug("End cb-network agent .........")
}
