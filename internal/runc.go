package internal

import (
	"encoding/json"
	"fmt"
	"strings"

	compose "github.com/compose-spec/compose-go/types"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/strslice"
	"github.com/docker/docker/oci"
	"github.com/docker/docker/pkg/system"
)

/*func exposedPorts(ports []compose.ServicePortConfig) nat.PortSet {
	natPorts := nat.PortSet{}
	for _, p := range ports
		p := nat.Port(fmt.Sprintf("%d/%s", p.Target, p.Protocol))
		natPorts[p] = struct{}{}
	}
	return natPorts
}*/

func env(s compose.ServiceConfig, e types.ExecConfig) []string {
	envMap := make(map[string]*string)

	// first populate map from execConfig
	for _, val := range e.Env {
		parts := strings.SplitN(val, "=", 2)
		if len(parts) == 1 {
			envMap[parts[0]] = nil
		} else {
			envMap[parts[0]] = &parts[1]
		}
	}

	// now override with service vals
	for k, v := range s.Environment {
		envMap[k] = v
	}

	// pull in special vals:
	if s.Tty {
		if _, ok := envMap["TERM"]; !ok {
			term := "xterm"
			envMap["TERM"] = &term
		}
	}
	if _, ok := envMap["PATH"]; !ok {
		path := system.DefaultPathEnv("linux")
		envMap["PATH"] = &path
	}
	if _, ok := envMap["HOSTNAME"]; !ok {
		envMap["HOSTNAME"] = &s.Hostname
	}

	env := make([]string, 0, len(envMap))
	for k, v := range envMap {
		if v != nil {
			env = append(env, k+"="+*v)
		} else {
			env = append(env, k)
		}
	}
	return env
}

func command(s compose.ServiceConfig, e types.ExecConfig) []string {
	if len(s.Command) > 0 {
		return s.Command
	}
	return e.Cmd
}

func workingDir(s compose.ServiceConfig, e types.ExecConfig) string {
	if len(s.WorkingDir) > 0 {
		return s.WorkingDir
	}
	if len(e.WorkingDir) > 0 {
		return e.WorkingDir
	}
	return "/"
}

func user(s compose.ServiceConfig, e types.ExecConfig) string {
	if len(s.User) > 0 {
		return s.User
	}
	return e.User
}

func RuncSpec(s compose.ServiceConfig, containerConfig []byte) ([]byte, error) {
	var execConfig types.ExecConfig
	if err := json.Unmarshal(containerConfig, &execConfig); err != nil {
		return nil, err
	}
	labels := map[string]string{}
	for k, v := range s.Labels {
		labels[k] = v
	}
	labels["io.compose-spec.project"] = "TODO"
	labels["io.compose-spec.service"] = s.Name

	// https://github.com/moby/moby/blob/470ae8422fc6f1845288eb7572253b08f1e6edf8/daemon/oci_linux.go
	// https://github.com/opencontainers/runtime-spec/blob/master/specs-go/config.go
	/*fmt.Println("=========")
	fmt.Println("OpenStdin:", s.StdinOpen)
	fmt.Println("Image:", s.Image)
	fmt.Println("Labels:", labels)
	fmt.Println("Entrypoint:", strslice.StrSlice(s.Entrypoint))
	fmt.Println("NetworkDisabled:", s.NetworkMode == "disabled")
	fmt.Println("MacAddress:", s.MacAddress)
	fmt.Println("StopSignal:", s.StopSignal)
	fmt.Println("ExposedPorts:", ExposedPorts(s.Ports))*/

	spec := oci.DefaultSpec()
	spec.Hostname = s.Hostname
	if s.DomainName != "" {
		spec.Linux.Sysctl = make(map[string]string)
		spec.Linux.Sysctl["kernel.domainname"] = s.DomainName
	}
	// TODO fill out all of spec.user
	spec.Process.User.Username = user(s, execConfig)
	spec.Process.Terminal = s.Tty
	spec.Process.Args = strslice.StrSlice(command(s, execConfig))
	spec.Process.Cwd = workingDir(s, execConfig)
	spec.Process.Env = env(s, execConfig)
	/*&container.HostConfig{
		NetworkMode:    networkMode,
		RestartPolicy:  container.RestartPolicy{Name: s.Restart},
		CapAdd:         s.CapAdd,
		CapDrop:        s.CapDrop,
		DNS:            s.DNS,
		DNSSearch:      s.DNSSearch,
		ExtraHosts:     s.ExtraHosts,
		IpcMode:        container.IpcMode(s.Ipc),
		Links:          s.Links,
		Mounts:         mounts,
		PidMode:        container.PidMode(s.Pid),
		Privileged:     s.Privileged,
		ReadonlyRootfs: s.ReadOnly,
		SecurityOpt:    s.SecurityOpt,
		UsernsMode:     container.UsernsMode(s.UserNSMode),
		ShmSize:        shmSize,
		Sysctls:        s.Sysctls,
		Isolation:      container.Isolation(s.Isolation),
		Init:           s.Init,
		PortBindings:   internal.BuildContainerPortBindingsOptions(s),
	},
	*/
	return json.MarshalIndent(spec, "", "  ")
}

func CreateSpecs(proj *compose.Project, configs ServiceConfigs) (map[string][]byte, error) {
	specs := make(map[string][]byte)
	return specs, proj.WithServices(nil, func(s compose.ServiceConfig) error {
		for _, containerConfig := range configs[s.Name] {
			fname := s.Name + "/"
			if len(containerConfig.Platform) == 0 {
				fname += "default"
			} else {
				fname += containerConfig.Platform
			}
			fmt.Println("Creating runc spec for", fname)
			spec, err := RuncSpec(s, containerConfig.Config)
			if err != nil {
				return err
			}
			specs[fname] = spec
		}
		return nil
	})
}
