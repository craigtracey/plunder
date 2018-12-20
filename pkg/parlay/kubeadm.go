package parlay

import (
	"fmt"
)

// NOTE - The functions in this particluar file will need moving to something seperate at a later
// date. Quite possibly moving to a plugin model? TBD.

// This defines the etcd kubeadm file (should use the kubernetes packages to define at a later point)
const etcdKubeadm = `apiVersion: "kubeadm.k8s.io/%s"
kind: ClusterConfiguration
etcd:
    local:
        serverCertSANs:
        - "%s"
        peerCertSANs:
        - "%s"
        extraArgs:
            initial-cluster: %s=https://%s:2380,%s=https://%s:2380,%s=https://%s:2380
            initial-cluster-state: new
            name: %s
            listen-peer-urls: https://%s:2380
            listen-client-urls: https://%s:2379
            advertise-client-urls: https://%s:2379
            initial-advertise-peer-urls: https://%s:2380`

type etcdMembers struct {
	// Hostnames
	Hostname1 string `json:"hostname1,omitempty"`
	Hostname2 string `json:"hostname2,omitempty"`
	Hostname3 string `json:"hostname3,omitempty"`

	// Addresses
	Address1 string `json:"address1,omitempty"`
	Address2 string `json:"address2,omitempty"`
	Address3 string `json:"address3,omitempty"`

	// Intialise a Certificate Authority
	InitCA bool `json:"initCA,omitempty"`

	// Set kubernetes API version
	APIVersion string `json:"apiversion,omitempty"`
}

func (e *etcdMembers) generateActions() []Action {
	var generatedActions []Action
	var a Action
	if e.InitCA == true {
		// Ensure that a new Certificate Authority is generated
		// Create action
		a := Action{
			// Generate etcd server certificate
			ActionType:  "command",
			Command:     fmt.Sprintf("kubeadm init phase certs etcd-ca"),
			CommandSudo: "root",
			Name:        "Initialise Certificate Authority",
		}
		generatedActions = append(generatedActions, a)
	}

	// Default to < 1.12 API version
	if e.APIVersion == "" {
		e.APIVersion = "v1beta1"
	}
	// Generate the configuration directories
	a.ActionType = "command"
	a.Command = fmt.Sprintf("mkdir -p /tmp/%s/ /tmp/%s/ /tmp/%s/", e.Address1, e.Address2, e.Address3)
	a.Name = "Generate temporary directories"
	generatedActions = append(generatedActions, a)

	// Generate the kubeadm configuration files

	// Node 0
	a.Name = "build kubeadm config for node 0"
	a.Command = fmt.Sprintf("echo '%s' > /tmp/%s/kubeadmcfg.yaml", e.buildKubeadm(e.APIVersion, e.Hostname1, e.Address1), e.Address1)
	generatedActions = append(generatedActions, a)

	// Node 1
	a.Name = "build kubeadm config for node 1"
	a.Command = fmt.Sprintf("echo '%s' > /tmp/%s/kubeadmcfg.yaml", e.buildKubeadm(e.APIVersion, e.Hostname2, e.Address2), e.Address2)
	generatedActions = append(generatedActions, a)

	// Node 2
	a.Command = fmt.Sprintf("echo '%s' > /tmp/%s/kubeadmcfg.yaml", e.buildKubeadm(e.APIVersion, e.Hostname3, e.Address3), e.Address3)
	generatedActions = append(generatedActions, a)

	// Add certificate actions
	generatedActions = append(generatedActions, e.generateCertificateActions([]string{e.Address3, e.Address2, e.Address1})...)
	return generatedActions
}

func (e *etcdMembers) buildKubeadm(api, host, address string) string {
	var kubeadm string
	// Generates a kubeadm for setting up the etcd yaml
	kubeadm = fmt.Sprintf(etcdKubeadm, api, address, address, e.Hostname1, e.Address1, e.Hostname2, e.Address2, e.Hostname3, e.Address3, host, address, address, address, address)
	return kubeadm
}

// generateCertificateActions - Hosts need adding in backward to the array i.e. host 2 -> host 1 -> host 0
func (e *etcdMembers) generateCertificateActions(hosts []string) []Action {
	var generatedActions []Action
	for i, v := range hosts {
		// Create action variable
		a := Action{
			// Tidy the certificates from the /etc/kubernetes/pki folder
			ActionType:  "command",
			Command:     fmt.Sprintf("find /etc/kubernetes/pki -not -name ca.crt -not -name ca.key -type f -delete"),
			CommandSudo: "root",
			Name:        fmt.Sprintf("Remove any existing certificates before attempting to generate any new ones"),
		}
		generatedActions = append(generatedActions, a)

		// Generate etcd server certificate
		a.ActionType = "command"
		a.Command = fmt.Sprintf("kubeadm init phase certs etcd-server --config=/tmp/%s/kubeadmcfg.yaml", v)
		a.Name = fmt.Sprintf("Generate etcd server certificate for [%s]", v)
		generatedActions = append(generatedActions, a)

		// Generate peer certificate
		a.Command = fmt.Sprintf("kubeadm init phase certs etcd-peer --config=/tmp/%s/kubeadmcfg.yaml", v)
		a.Name = fmt.Sprintf("Generate peer certificate for [%s]", v)
		generatedActions = append(generatedActions, a)

		// Generate health check certificate
		a.Command = fmt.Sprintf("kubeadm init phase certs etcd-healthcheck-client --config=/tmp/%s/kubeadmcfg.yaml", v)
		a.Name = fmt.Sprintf("Generate health check certificate for [%s]", v)
		generatedActions = append(generatedActions, a)

		// Generate api-server client certificate
		a.Command = fmt.Sprintf("kubeadm init phase certs apiserver-etcd-client --config=/tmp/%s/kubeadmcfg.yaml", v)
		a.Name = fmt.Sprintf("Generate api-server client certificate for [%s]", v)
		generatedActions = append(generatedActions, a)

		// These steps are only required for the latter two hosts
		if i != (len(hosts) - 1) {
			// Archive the certificates and the kubeadm configuration in a host specific archive name
			a.Command = fmt.Sprintf("tar -cvzf /tmp/%s.tar.gz $(find /etc/kubernetes/pki -not -name ca.crt -not -name ca.key -type f) /tmp/%s/kubeadmcfg.yaml", v, v)
			a.Name = fmt.Sprintf("Archive generated certificates [%s]", v)
			generatedActions = append(generatedActions, a)

			// Download the archive files to the local machine
			a.ActionType = "download"
			a.Source = fmt.Sprintf("/tmp/%s.tar.gz", hosts[i])
			a.Destination = fmt.Sprintf("/tmp/%s.tar.gz", hosts[i])
			a.Name = fmt.Sprintf("Retrieve the certificate bundle for [%s]", v)
			generatedActions = append(generatedActions, a)
		}
	}
	return generatedActions
}