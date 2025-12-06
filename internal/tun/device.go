package tun

import (
	"fmt"
	"log"
	"os/exec"

	"github.com/songgao/water"
)

// Setup creates and configures the TUN interface
func Setup(ip string) (*water.Interface, error) {
	config := water.Config{DeviceType: water.TUN}
	ifce, err := water.New(config)
	if err != nil {
		return nil, err
	}

	log.Printf("[TUN] Interface %s created", ifce.Name())
	
	if err := configureInterface(ifce.Name(), ip); err != nil {
		return nil, err
	}

	return ifce, nil
}

// configureInterface runs Linux ip commands to set address and MTU
func configureInterface(ifaceName, ip string) error {
	cidr := ip + "/24"
	
	// Assign IP
	if err := runCmd("ip", "addr", "add", cidr, "dev", ifaceName); err != nil {
		log.Printf("[TUN] Note: IP assignment might already exist: %v", err)
	}

	// Set MTU (1300 is safe for UDP encapsulation)
	if err := runCmd("ip", "link", "set", "dev", ifaceName, "mtu", "1300"); err != nil {
		return fmt.Errorf("failed to set MTU: %v", err)
	}

	// Set UP
	if err := runCmd("ip", "link", "set", "dev", ifaceName, "up"); err != nil {
		return fmt.Errorf("failed to bring up interface: %v", err)
	}

	return nil
}

func runCmd(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}