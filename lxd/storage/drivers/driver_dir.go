package drivers

import (
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/lxc/lxd/lxd/operations"
	"github.com/lxc/lxd/shared"
	"github.com/lxc/lxd/shared/api"
)

type dir struct {
	common
}

// Info returns info about the driver and its environment.
func (d *dir) Info() Info {
	return Info{
		Name:                  "dir",
		Version:               "1",
		OptimizedImages:       false,
		PreservesInodes:       false,
		Remote:                false,
		VolumeTypes:           []VolumeType{VolumeTypeCustom, VolumeTypeImage, VolumeTypeContainer, VolumeTypeVM},
		BlockBacking:          false,
		RunningQuotaResize:    true,
		RunningSnapshotFreeze: true,
	}
}

// Create is called during pool creation and is effectively using an empty driver struct.
// WARNING: The Create() function cannot rely on any of the struct attributes being set.
func (d *dir) Create() error {
	// Set default source if missing.
	if d.config["source"] == "" {
		d.config["source"] = GetPoolMountPath(d.name)
	}

	if !shared.PathExists(d.config["source"]) {
		return fmt.Errorf("Source path '%s' doesn't exist", d.config["source"])
	}

	// Check that if within LXD_DIR, we're at our expected spot.
	cleanSource := filepath.Clean(d.config["source"])
	if strings.HasPrefix(cleanSource, shared.VarPath()) && cleanSource != GetPoolMountPath(d.name) {
		return fmt.Errorf("Source path '%s' is within the LXD directory", d.config["source"])
	}

	// Check that the path is currently empty.
	isEmpty, err := shared.PathIsEmpty(d.config["source"])
	if err != nil {
		return err
	}

	if !isEmpty {
		return fmt.Errorf("Source path '%s' isn't empty", d.config["source"])
	}

	return nil
}

// Delete removes the storage pool from the storage device.
func (d *dir) Delete(op *operations.Operation) error {
	// On delete, wipe everything in the directory.
	err := wipeDirectory(GetPoolMountPath(d.name))
	if err != nil {
		return err
	}

	// Unmount the path.
	_, err = d.Unmount()
	if err != nil {
		return err
	}

	return nil
}

// Validate checks that all provide keys are supported and that no conflicting or missing configuration is present.
func (d *dir) Validate(config map[string]string) error {
	return nil
}

// Update applies any driver changes required from a configuration change.
func (d *dir) Update(changedConfig map[string]string) error {
	return nil
}

// Mount mounts the storage pool.
func (d *dir) Mount() (bool, error) {
	path := GetPoolMountPath(d.name)

	// Check if we're dealing with an external mount.
	if d.config["source"] == path {
		return false, nil
	}

	// Check if already mounted.
	if sameMount(d.config["source"], path) {
		return false, nil
	}

	// Setup the bind-mount.
	err := TryMount(d.config["source"], path, "none", unix.MS_BIND, "")
	if err != nil {
		return false, err
	}

	return true, nil
}

// Unmount unmounts the storage pool.
func (d *dir) Unmount() (bool, error) {
	path := GetPoolMountPath(d.name)

	// Check if we're dealing with an external mount.
	if d.config["source"] == path {
		return false, nil
	}

	// Unmount until nothing is left mounted.
	return forceUnmount(path)
}

// GetResources returns the pool resource usage information.
func (d *dir) GetResources() (*api.ResourcesStoragePool, error) {
	return d.vfsGetResources()
}
