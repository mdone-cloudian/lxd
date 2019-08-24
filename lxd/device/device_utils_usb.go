package device

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/lxc/lxd/lxd/device/config"
	"github.com/lxc/lxd/lxd/state"
	log "github.com/lxc/lxd/shared/log15"
	"github.com/lxc/lxd/shared/logger"
)

// usbDevPath is the path where USB devices can be enumerated.
const usbDevPath = "/sys/bus/usb/devices"

// USBDevice represents the properties of a USB device and a USB uevent.
type USBDevice struct {
	Action string

	Vendor  string
	Product string

	Path        string
	Major       uint32
	Minor       uint32
	UeventParts []string
	UeventLen   int
}

// usbHandlers stores the event handler callbacks for USB events.
var usbHandlers = map[string]func(USBDevice) (*RunConfig, error){}

// usbMutex controls access to the usbHandlers map.
var usbMutex sync.Mutex

// USBRegisterHandler registers a handler function to be called whenever a USB device event occurs.
func USBRegisterHandler(instance InstanceIdentifier, deviceName string, handler func(USBDevice) (*RunConfig, error)) {
	usbMutex.Lock()
	defer usbMutex.Unlock()

	// Null delimited string of project name, instance name and device name.
	key := fmt.Sprintf("%s\000%s\000%s", instance.Project(), instance.Name(), deviceName)
	usbHandlers[key] = handler
}

// USBUnregisterHandler removes a registered USB handler function for a device.
func USBUnregisterHandler(instance InstanceIdentifier, deviceName string) {
	usbMutex.Lock()
	defer usbMutex.Unlock()

	// Null delimited string of project name, instance name and device name.
	key := fmt.Sprintf("%s\000%s\000%s", instance.Project(), instance.Name(), deviceName)
	delete(usbHandlers, key)
}

// USBRunHandlers executes any handlers registered for USB events.
func USBRunHandlers(state *state.State, event *USBDevice) {
	usbMutex.Lock()
	defer usbMutex.Unlock()

	for key, hook := range usbHandlers {
		keyParts := strings.SplitN(key, "\000", 3)
		projectName := keyParts[0]
		instanceName := keyParts[1]
		deviceName := keyParts[2]

		if hook == nil {
			delete(usbHandlers, key)
			continue
		}

		runConf, err := hook(*event)
		if err != nil {
			logger.Error("USB event hook failed", log.Ctx{"err": err, "project": projectName, "instance": instanceName, "device": deviceName})
			continue
		}

		// If runConf supplied, load instance and call its USB event handler function so
		// any instance specific device actions can occur.
		if runConf != nil {
			instance, err := InstanceLoadByProjectAndName(state, projectName, instanceName)
			if err != nil {
				logger.Error("USB event loading instance failed", log.Ctx{"err": err, "project": projectName, "instance": instanceName, "device": deviceName})
				continue
			}

			err = instance.DeviceEventHandler(runConf)
			if err != nil {
				logger.Error("USB event instance handler failed", log.Ctx{"err": err, "project": projectName, "instance": instanceName, "device": deviceName})
				continue
			}
		}
	}
}

// USBDeviceLoad instantiates a new USBDevice struct.
func USBDeviceLoad(action string, vendor string, product string, major string, minor string, busnum string, devnum string, devname string, ueventParts []string, ueventLen int) (USBDevice, error) {
	majorInt, err := strconv.ParseUint(major, 10, 32)
	if err != nil {
		return USBDevice{}, err
	}

	minorInt, err := strconv.ParseUint(minor, 10, 32)
	if err != nil {
		return USBDevice{}, err
	}

	path := devname
	if devname == "" {
		busnumInt, err := strconv.Atoi(busnum)
		if err != nil {
			return USBDevice{}, err
		}

		devnumInt, err := strconv.Atoi(devnum)
		if err != nil {
			return USBDevice{}, err
		}
		path = fmt.Sprintf("/dev/bus/usb/%03d/%03d", busnumInt, devnumInt)
	} else {
		if !filepath.IsAbs(devname) {
			path = fmt.Sprintf("/dev/%s", devname)
		}
	}

	return USBDevice{
		action,
		vendor,
		product,
		path,
		uint32(majorInt),
		uint32(minorInt),
		ueventParts,
		ueventLen,
	}, nil
}

// USBIsOurDevice indicates whether the USB device event qualifies as part of our device.
func USBIsOurDevice(config config.Device, usb *USBDevice) bool {
	// Check if event matches criteria for this device, if not return.
	if (config["vendorid"] != "" && config["vendorid"] != usb.Vendor) || (config["productid"] != "" && config["productid"] != usb.Product) {
		return false
	}

	return true
}