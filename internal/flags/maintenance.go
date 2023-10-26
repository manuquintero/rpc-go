package flags

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"rpc/internal/amt"
	"rpc/pkg/utils"

	log "github.com/sirupsen/logrus"
)

func (f *Flags) printMaintenanceUsage() string {
	executable := filepath.Base(os.Args[0])
	usage := "\nRemote Provisioning Client (RPC) - used for activation, deactivation, maintenance and status of AMT\n\n"
	usage = usage + "Usage: " + executable + " maintenance COMMAND [OPTIONS]\n\n"
	usage = usage + "Supported Maintenance Commands:\n"
	usage = usage + "  changepassword Change the AMT password. A random password is generated by default. Specify -static to set manually. AMT password is required\n"
	usage = usage + "                 Example: " + executable + " maintenance changepassword -u wss://server/activate\n"
	usage = usage + "  syncdeviceinfo Sync device information. AMT password is required\n"
	usage = usage + "                 Example: " + executable + " maintenance syncdeviceinfo -u wss://server/activate\n"
	usage = usage + "  syncclock      Sync the host OS clock to AMT. AMT password is required\n"
	usage = usage + "                 Example: " + executable + " maintenance syncclock -u wss://server/activate\n"
	usage = usage + "  synchostname   Sync the hostname of the client to AMT. AMT password is required\n"
	usage = usage + "                 Example: " + executable + " maintenance synchostname -u wss://server/activate\n"
	usage = usage + "  syncip         Sync the IP configuration of the host OS to AMT Network Settings. AMT password is required\n"
	usage = usage + "                 Example: " + executable + " maintenance syncip -staticip 192.168.1.7 -netmask 255.255.255.0 -gateway 192.168.1.1 -primarydns 8.8.8.8 -secondarydns 4.4.4.4 -u wss://server/activate\n"
	usage = usage + "                 If a static ip is not specified, the ip address and netmask of the host OS is used\n"
	usage = usage + "\nRun '" + executable + " maintenance COMMAND -h' for more information on a command.\n"
	fmt.Println(usage)
	return usage
}

func (f *Flags) handleMaintenanceCommand() utils.ReturnCode {
	//validation section
	if len(f.commandLineArgs) == 2 {
		f.printMaintenanceUsage()
		return utils.IncorrectCommandLineParameters
	}

	var rc = utils.Success

	f.SubCommand = f.commandLineArgs[2]
	switch f.SubCommand {
	case "syncclock":
		rc = f.handleMaintenanceSyncClock()
		break
	case "synchostname":
		rc = f.handleMaintenanceSyncHostname()
		break
	case "syncip":
		rc = f.handleMaintenanceSyncIP()
		break
	case "changepassword":
		rc = f.handleMaintenanceSyncChangePassword()
		break
	case "syncdeviceinfo":
		rc = f.handleMaintenanceSyncDeviceInfo()
		break
	default:
		f.printMaintenanceUsage()
		rc = utils.IncorrectCommandLineParameters
		break
	}
	if rc != utils.Success {
		return rc
	}

	if f.Password == "" {
		if _, rc := f.ReadPasswordFromUser(); rc != 0 {
			return utils.MissingOrIncorrectPassword
		}
	}
	f.LocalConfig.Password = f.Password

	// if this is a local command, then we dont care about -u or what task/command since its not going to the cloud
	if !f.Local {
		if f.URL == "" {
			fmt.Print("\n-u flag is required and cannot be empty\n\n")
			f.printMaintenanceUsage()
			return utils.MissingOrIncorrectURL
		}
	}

	return utils.Success
}

func (f *Flags) handleMaintenanceSyncClock() utils.ReturnCode {
	if err := f.amtMaintenanceSyncClockCommand.Parse(f.commandLineArgs[3:]); err != nil {
		return utils.IncorrectCommandLineParameters
	}
	return utils.Success
}

func (f *Flags) handleMaintenanceSyncDeviceInfo() utils.ReturnCode {
	if err := f.amtMaintenanceSyncDeviceInfoCommand.Parse(f.commandLineArgs[3:]); err != nil {
		return utils.IncorrectCommandLineParameters
	}
	return utils.Success
}

func (f *Flags) handleMaintenanceSyncHostname() utils.ReturnCode {
	var err error
	if err = f.amtMaintenanceSyncHostnameCommand.Parse(f.commandLineArgs[3:]); err != nil {
		f.amtMaintenanceSyncHostnameCommand.Usage()
		return utils.IncorrectCommandLineParameters
	}
	amtCommand := amt.NewAMTCommand()
	if f.HostnameInfo.DnsSuffixOS, err = amtCommand.GetOSDNSSuffix(); err != nil {
		log.Error(err)
	}
	f.HostnameInfo.Hostname, err = os.Hostname()
	if err != nil {
		log.Error(err)
		return utils.OSNetworkInterfacesLookupFailed
	} else if f.HostnameInfo.Hostname == "" {
		log.Error("OS hostname is not available")
		return utils.OSNetworkInterfacesLookupFailed
	}
	return utils.Success
}

// wrap the flag.Func method signature with the assignment value
func validateIP(assignee *string) func(string) error {
	return func(val string) error {
		if net.ParseIP(val) == nil {
			return errors.New("not a valid ip address")
		}
		*assignee = val
		return nil
	}
}

func (f *Flags) handleMaintenanceSyncIP() utils.ReturnCode {
	f.amtMaintenanceSyncIPCommand.Func(
		"staticip",
		"IP address to be assigned to AMT - if not specified, the IP Address of the active OS newtork interface is used",
		validateIP(&f.IpConfiguration.IpAddress))
	f.amtMaintenanceSyncIPCommand.Func(
		"netmask",
		"Network mask to be assigned to AMT - if not specified, the Network mask of the active OS newtork interface is used",
		validateIP(&f.IpConfiguration.Netmask))
	f.amtMaintenanceSyncIPCommand.Func("gateway", "Gateway address to be assigned to AMT", validateIP(&f.IpConfiguration.Gateway))
	f.amtMaintenanceSyncIPCommand.Func("primarydns", "Primary DNS to be assigned to AMT", validateIP(&f.IpConfiguration.PrimaryDns))
	f.amtMaintenanceSyncIPCommand.Func("secondarydns", "Secondary DNS to be assigned to AMT", validateIP(&f.IpConfiguration.SecondaryDns))

	if err := f.amtMaintenanceSyncIPCommand.Parse(f.commandLineArgs[3:]); err != nil {
		f.amtMaintenanceSyncIPCommand.Usage()
		// Parse the error message to find the problematic flag.
		// The problematic flag is of the following format '-' followed by flag name and then a ':'
		var rc utils.ReturnCode
		re := regexp.MustCompile(`-.*:`)
		switch re.FindString(err.Error()) {
		case "-netmask:":
			rc = utils.MissingOrIncorrectNetworkMask
		case "-staticip:":
			rc = utils.MissingOrIncorrectStaticIP
		case "-gateway:":
			rc = utils.MissingOrIncorrectGateway
		case "-primarydns:":
			rc = utils.MissingOrIncorrectPrimaryDNS
		case "-secondarydns:":
			rc = utils.MissingOrIncorrectSecondaryDNS
		default:
			rc = utils.IncorrectCommandLineParameters
		}
		return rc
	} else if len(f.IpConfiguration.IpAddress) != 0 {
		return utils.Success
	}

	amtLanIfc, err := f.amtCommand.GetLANInterfaceSettings(false)
	if err != nil {
		log.Error(err)
		return utils.AMTConnectionFailed
	}

	ifaces, err := f.netEnumerator.Interfaces()
	if err != nil {
		log.Error(err)
		return utils.OSNetworkInterfacesLookupFailed
	}

	for _, i := range ifaces {
		if len(f.IpConfiguration.IpAddress) != 0 {
			break
		}
		if i.HardwareAddr.String() != amtLanIfc.MACAddress {
			continue
		}
		addrs, _ := f.netEnumerator.InterfaceAddrs(&i)
		if err != nil {
			continue
		}
		for _, address := range addrs {
			if ipnet, ok := address.(*net.IPNet); ok &&
				ipnet.IP.To4() != nil &&
				!ipnet.IP.IsLoopback() {
				f.IpConfiguration.IpAddress = ipnet.IP.String()
				f.IpConfiguration.Netmask = net.IP(ipnet.Mask).String()
			}
		}
	}

	if len(f.IpConfiguration.IpAddress) == 0 {
		log.Errorf("static ip address not found")
		return utils.OSNetworkInterfacesLookupFailed
	}
	return utils.Success
}

func (f *Flags) handleMaintenanceSyncChangePassword() utils.ReturnCode {
	f.amtMaintenanceChangePasswordCommand.StringVar(&f.StaticPassword, "static", "", "specify a new password for AMT")
	if err := f.amtMaintenanceChangePasswordCommand.Parse(f.commandLineArgs[3:]); err != nil {
		f.amtMaintenanceChangePasswordCommand.Usage()
		return utils.IncorrectCommandLineParameters
	}
	return utils.Success
}
