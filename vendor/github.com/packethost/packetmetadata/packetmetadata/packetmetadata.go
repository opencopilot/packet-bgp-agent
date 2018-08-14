package packetmetadata

import (
	"context"
	"crypto/x509"
	"encoding/json"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/packethost/packetmetadata/hegel"
	"github.com/packethost/packngo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var hegelAddr = "metadata.packet.net:50060"

// Metadata is a struct the json response can be serialized into
type Metadata struct {
	ID                                 string                   `json:"id"`
	State                              string                   `json:"state"`
	Instance                           *packngo.Device          `json:"instance"`
	PreinstalledOperatingSystemVersion interface{}              `json:"preinstalled_operating_system_version"`
	NetworkPorts                       []map[string]interface{} `json:"network_ports"`
	PlanSlug                           string                   `json:"plan_slug"`
	Facility                           string                   `json:"facility_code"`
	Hostname                           string                   `json:"hostname"`
	BondingMode                        int                      `json:"bonding_mode"`
}

// WatchResult represents a change in metadata
type WatchResult struct {
	JSON     []byte
	Patch    []byte
	Metadata *Metadata
}

// WatchIterator is a struct for iterating over watch results
type WatchIterator struct {
	Next  func() (*WatchResult, error)
	Close func() error
}

func getHegelClient() (hegel.HegelClient, func() error, error) {
	certPool, err := x509.SystemCertPool()
	if err != nil {
		return nil, nil, err
	}
	cred := credentials.NewClientTLSFromCert(certPool, "")
	conn, err := grpc.Dial(hegelAddr, grpc.WithTransportCredentials(cred))
	if err != nil {
		return nil, nil, err
	}
	return hegel.NewHegelClient(conn), conn.Close, nil
}

// Get returns instance metadata from hegel
func Get() ([]byte, *Metadata, error) {
	hegelClient, close, err := getHegelClient()
	if err != nil {
		return nil, nil, err
	}
	defer close()
	res, err := hegelClient.Get(context.Background(), &hegel.GetRequest{})
	if err != nil {
		return nil, nil, err
	}

	var metadata *Metadata
	err = json.Unmarshal([]byte(res.JSON), &metadata)
	if err != nil {
		return nil, nil, err
	}
	return []byte(res.JSON), metadata, nil
}

// Watch returns an iterator of change events
func Watch() (*WatchIterator, error) {
	ctx := context.Background()

	watchResults := make(chan *WatchResult, 10)
	errorChan := make(chan error, 1)

	hegelClient, close, err := getHegelClient()
	if err != nil {
		return nil, err
	}

	res, err := hegelClient.Get(ctx, &hegel.GetRequest{})
	if err != nil {
		return nil, err
	}
	currentState := []byte(res.JSON)

	var metadata *Metadata
	err = json.Unmarshal(currentState, &metadata)
	if err != nil {
		return nil, err
	}
	watchResults <- &WatchResult{
		JSON:     currentState,
		Patch:    nil,
		Metadata: metadata,
	}

	client, err := hegelClient.Subscribe(ctx, &hegel.SubscribeRequest{})
	if err != nil {
		return nil, err
	}

	stopChan := make(chan bool, 1)
	go func(stopChan chan bool) {
		for {
			newResponse, err := client.Recv()
			if err != nil {
				errorChan <- err
				break
			}

			newState := []byte(newResponse.JSON)

			if !jsonpatch.Equal(currentState, newState) {
				patch, err := jsonpatch.CreateMergePatch(currentState, newState)
				if err != nil {
					errorChan <- err
				}

				var metadata *Metadata
				err = json.Unmarshal(currentState, &metadata)
				if err != nil {
					errorChan <- err
				}
				watchResults <- &WatchResult{
					JSON:     []byte(newResponse.JSON),
					Patch:    patch,
					Metadata: metadata,
				}
				currentState = newState
			}
		}
	}(stopChan)

	iterator := &WatchIterator{
		Next: func() (*WatchResult, error) {
			select {
			case err := <-errorChan:
				return nil, err
			case latest := <-watchResults:
				return latest, nil
			}
		},
		Close: func() error {
			stopChan <- true
			return close()
		},
	}

	return iterator, nil
}
