// Copyright 2012-2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pci

import (
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

// PCI is a PCI device. We will fill this in as we add options.
// For now it just holds two uint16 per the PCI spec.
type PCI struct {
	Addr       string
	Vendor     string `pci:"vendor"`
	Device     string `pci:"device"`
	VendorName string
	DeviceName string
	FullPath   string
	ExtraInfo  []string
	Config     []byte
	// The rest only gets filled in config space is read.
	Control Control
	Status  Status
}

// Control configures how the device responds to operations. It is the 3rd 16-bit word.
type Control uint16

// Status contains status bits for the PCI device. It is the 4th 16-bit word.
type Status uint16

// String concatenates PCI address, Vendor, and Device and other information
// to make a useful display for the user.
func (p *PCI) String() string {
	return strings.Join(append([]string{fmt.Sprintf("%s: %v %v", p.Addr, p.VendorName, p.DeviceName)}, p.ExtraInfo...), "\n")
}

// SetVendorDeviceName changes VendorName and DeviceName from a name to a number,
// if possible.
func (p *PCI) SetVendorDeviceName() {
	ids = newIDs()
	p.VendorName, p.DeviceName = Lookup(ids, p.Vendor, p.Device)
}

// ReadConfig reads the config space and adds it to ExtraInfo as a hexdump.
func (p *PCI) ReadConfig(n int) error {
	dev := filepath.Join(p.FullPath, "config")
	c, err := ioutil.ReadFile(dev)
	if err != nil {
		return err

	}
	// If we want more than 64 bytes, we MUST have read that or we're not
	// uid(0)
	if n > 64 && len(c) <= 64 {
		return fmt.Errorf("Read %q for %d bytes: %v (do you need to be root?)", dev, n, os.ErrPermission)
	}
	if n < len(c) {
		c = c[:n]
	}
	p.Config = c
	p.Control = Control(binary.LittleEndian.Uint16(c[4:6]))
	p.Status = Status(binary.LittleEndian.Uint16(c[6:8]))
	return nil
}

type barreg struct {
	offset int64
	*os.File
}

func (r *barreg) Read(b []byte) (int, error) {
	return r.ReadAt(b, r.offset)
}

func (r *barreg) Write(b []byte) (int, error) {
	return r.WriteAt(b, r.offset)
}

// ReadConfigRegister reads a configuration register of size 8, 16, 32, or 64.
// It will only work on little-endian machines.
func (p *PCI) ReadConfigRegister(offset, size int64) (uint64, error) {
	dev := filepath.Join(p.FullPath, "config")
	f, err := os.Open(dev)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	var reg uint64
	r := &barreg{offset: offset, File: f}
	switch size {
	default:
		return 0, fmt.Errorf("%d is not valid: only options are 8, 16, 32, 64", size)
	case 64:
		err = binary.Read(r, binary.LittleEndian, &reg)
	case 32:
		var val uint32
		err = binary.Read(r, binary.LittleEndian, &val)
		reg = uint64(val)
	case 16:
		var val uint16
		err = binary.Read(r, binary.LittleEndian, &val)
		reg = uint64(val)
	case 8:
		var val uint8
		err = binary.Read(r, binary.LittleEndian, &val)
		reg = uint64(val)
	}
	return reg, err
}

// WriteConfigRegister writes a configuration register of size 8, 16, 32, or 64.
// It will only work on little-endian machines.
func (p *PCI) WriteConfigRegister(offset, size int64, val uint64) error {
	f, err := os.OpenFile(filepath.Join(p.FullPath, "config"), os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	w := &barreg{offset: offset, File: f}
	switch size {
	default:
		return fmt.Errorf("%d is not valid: only options are 8, 16, 32, 64", size)
	case 64:
		err = binary.Write(w, binary.LittleEndian, &val)
	case 32:
		var v = uint32(val)
		err = binary.Write(w, binary.LittleEndian, &v)
	case 16:
		var v = uint16(val)
		err = binary.Write(w, binary.LittleEndian, &v)
	case 8:
		var v = uint8(val)
		err = binary.Write(w, binary.LittleEndian, &v)
	}
	return err
}

// Read implements the BusReader interface for type bus. Iterating over each
// PCI bus device, and applying optional Filters to it.
func (bus *bus) Read(filters ...Filter) (Devices, error) {
	devices := make(Devices, 0, len(bus.Devices))
iter:
	for _, d := range bus.Devices {
		p, err := onePCI(d)
		if err != nil {
			return nil, err
		}
		p.Addr = filepath.Base(d)
		p.FullPath = d
		for _, f := range filters {
			if !f(p) {
				continue iter
			}
		}
		if bus.confSize > 0 {
			if err := p.ReadConfig(bus.confSize); err != nil {
				return nil, err
			}
			p.SetVendorDeviceName()
		}

		devices = append(devices, p)
	}
	return devices, nil
}
