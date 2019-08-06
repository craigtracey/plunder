package services

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"path/filepath"

	"github.com/plunder-app/plunder/pkg/utils"
	log "github.com/sirupsen/logrus"
)

// These strings container the generated iPXE details that are passed to the bootloader when the correct url is requested
var preseed, kickstart, defaultBoot, vsphere, reboot string

// controller Pointer for the config API endpoint handler
var controller *BootController

//DeploymentAPIPath returns the URI that is used to interact with the plunder Deployment API
func DeploymentAPIPath() string {
	return "/deployment"
}

//ConfigAPIPath returns the URI that is used to interact with the plunder Configuration API
func ConfigAPIPath() string {
	return "/config"
}

func (c BootController) generateBootTypeHanders(mux *http.ServeMux) {

	// Find the default configuration
	defaultConfig := findBootConfigForName("default")
	if defaultConfig != nil {
		defaultBoot = utils.IPXEPreeseed(*c.HTTPAddress, defaultConfig.Kernel, defaultConfig.Initrd, defaultConfig.Cmdline)
		mux.HandleFunc("/default.ipxe", rootHandler)
	} else {
		log.Warnf("Found [%d] configurations and no \"default\" configuration", len(c.BootConfigs))
	}

	// If a preeseed configuration has been configured then add it, and create a HTTP endpoint
	preeseedConfig := findBootConfigForName("preseed")
	if preeseedConfig != nil {
		preseed = utils.IPXEPreeseed(*c.HTTPAddress, preeseedConfig.Kernel, preeseedConfig.Initrd, preeseedConfig.Cmdline)

		mux.HandleFunc("/preseed.ipxe", preseedHandler)
	}

	// If a kickstart configuration has been configured then add it, and create a HTTP endpoint
	kickstartConfig := findBootConfigForName("kickstart")
	if kickstartConfig != nil {
		kickstart = utils.IPXEPreeseed(*c.HTTPAddress, kickstartConfig.Kernel, kickstartConfig.Initrd, kickstartConfig.Cmdline)
		mux.HandleFunc("/kickstart.ipxe", kickstartHandler)
	}

	// If a vsphereConfig configuration has been configured then add it, and create a HTTP endpoint
	vsphereConfig := findBootConfigForName("vsphere")
	if vsphereConfig != nil {
		vsphere = utils.IPXEVSphere(*c.HTTPAddress, vsphereConfig.Kernel, vsphereConfig.Cmdline)
		mux.HandleFunc("/vsphere.ipxe", vsphereHandler)
	}
}

func (c *BootController) serveHTTP() error {

	// Use of a Mux allows the redefinition of http paths
	mux := http.NewServeMux()

	// This function will pre-generate the boot handlers for the various boot types
	c.generateBootTypeHanders(mux)

	reboot = utils.IPXEReboot()

	docroot, err := filepath.Abs("./")
	if err != nil {
		return err
	}

	mux.Handle("/", http.FileServer(http.Dir(docroot)))

	mux.HandleFunc("/health", HealthCheckHandler)
	mux.HandleFunc("/reboot.ipxe", rebootHandler)

	// API Endpoints - allow the update of various configuration without restarting
	mux.HandleFunc(DeploymentAPIPath(), deploymentHandler)

	// Set the pointer to the boot config
	controller = c
	mux.HandleFunc(ConfigAPIPath(), configHandler)

	return http.ListenAndServe(":80", nil)
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	log.Debugf("Requested URL [%s]", r.RequestURI)

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "text/plain")
	// Return the preseed content
	log.Debugf("Requested URL [%s]", r.URL.Host)
	io.WriteString(w, httpPaths[r.URL.Path])
}

func preseedHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "text/plain")
	// Return the preseed content
	io.WriteString(w, preseed)
}

func kickstartHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "text/plain")
	// Return the kickstart content
	io.WriteString(w, kickstart)
}

func vsphereHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "text/plain")
	// Return the vsphere content
	io.WriteString(w, vsphere)
}

func defaultBootHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "text/plain")
	// Return the default boot content
	io.WriteString(w, defaultBoot)
}

func rebootHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "text/plain")
	// Return the reboot content
	io.WriteString(w, reboot)
}

// HealthCheckHandler -
func HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	// A very simple health check.
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")

	// In the future we could report back on the status of our DB, or our cache
	// (e.g. Redis) by performing a simple PING, and include them in the response.
	io.WriteString(w, `{"alive": true}`)
}

func deploymentHandler(w http.ResponseWriter, r *http.Request) {

	switch r.Method {
	case "GET":
		b, err := json.MarshalIndent(Deployments, "", "\t")
		if err != nil {
			io.WriteString(w, "<b>Unable to Parse Deployment configuration</b>")
			w.WriteHeader(http.StatusBadRequest)
			w.Header().Set("Content-Type", "application/json")
		}
		io.WriteString(w, string(b))
	case "POST":
		if b, err := ioutil.ReadAll(r.Body); err == nil {
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				w.Header().Set("Content-Type", "application/json")
				errorHTML := fmt.Sprintf("<b>Unable to Parse Deployment configuration</b>\n Error: %s", err.Error())
				io.WriteString(w, errorHTML)
			}
			err := UpdateDeploymentConfig(b)
			w.WriteHeader(http.StatusBadRequest)
			w.Header().Set("Content-Type", "application/json")
			if err != nil {
				errorHTML := fmt.Sprintf("<b>Unable to Parse Deployment configuration</b>\n Error: %s", err.Error())
				io.WriteString(w, errorHTML)
			}
		}
	default:
		// Unknown HTTP Method for this endpoint
	}
}

func configHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case "GET":
		b, err := json.MarshalIndent(controller, "", "\t")
		if err != nil {
			io.WriteString(w, "<b>Unable to Parse configuration</b>")
		}
		io.WriteString(w, string(b))
	case "POST":
		if _, err := ioutil.ReadAll(r.Body); err == nil {
			if err != nil {
				errorHTML := fmt.Sprintf("<b>Unable to Parse configuration</b>\n Error: %s", err.Error())
				io.WriteString(w, errorHTML)
			}

			// TODO - (thebsdbox) add updating of BootController

		}
	default:
		// Unknown HTTP Method for this endpoint
	}
}
