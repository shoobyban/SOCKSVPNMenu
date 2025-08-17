package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/getlantern/systray"
)

// Config: VPN configuration
type Config struct {
	AutoSSHPath   string        `json:"autossh_path"`
	LocalPort     int           `json:"local_port"`
	Interface     string        `json:"interface"`
	ServerOptions ServerOptions `json:"server_options"`
	Commands      []Command     `json:"commands"`
}

// ServerOptions: SSH keepalive options
type ServerOptions struct {
	ServerAliveInterval int `json:"server_alive_interval"`
	ServerAliveCountMax int `json:"server_alive_count_max"`
}

// Command: a named VPN server entry
type Command struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Server      string `json:"server"`
}

type VPNApp struct {
	config         *Config
	disconnectItem *systray.MenuItem
}

func main() {
	app := &VPNApp{}

	// Load configuration
	config, err := app.loadConfig()
	if err != nil {
		log.Printf("Warning: Could not load config: %v", err)
		// Create a minimal config for demo
		config = &Config{
			AutoSSHPath: "/opt/homebrew/bin/autossh",
			LocalPort:   1234,
			Interface:   "Wi-Fi",
			Commands:    []Command{},
		}
	}
	app.config = config

	systray.Run(app.onReady, app.onExit)
}

func (app *VPNApp) onReady() {
	// build menu, then set initial icon and start a periodic refresh
	systray.SetTooltip("SOCKS VPN Menu")
	app.buildMenu()
	app.updateIcon()

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			app.updateIcon()
		}
	}()
}

func (app *VPNApp) onExit() {
	// Disconnect VPN before exit
	app.disconnectVPN()
}

func (app *VPNApp) buildMenu() {
	// Status is shown in the menu bar icon; keep the menu minimal.

	// Add VPN commands
	for _, cmd := range app.config.Commands {
		item := systray.AddMenuItem(cmd.Description, fmt.Sprintf("Connect to %s", cmd.Server))
		go func(commandName string) {
			for {
				<-item.ClickedCh
				err := app.connectVPN(commandName)
				if err != nil {
					log.Printf("Error connecting to %s: %v", commandName, err)
				} else {
					log.Printf("Connected to %s", commandName)
					app.updateIcon()
					// Note: Cannot rebuild menu dynamically with this systray version
				}
			}
		}(cmd.Name)
	}

	if len(app.config.Commands) > 0 {
		systray.AddSeparator()
	}

	// Disconnect
	app.disconnectItem = systray.AddMenuItem("Disconnect", "Disconnect from VPN")
	if !app.isVPNConnected() {
		app.disconnectItem.Disable()
	}
	go func() {
		for {
			<-app.disconnectItem.ClickedCh

			// Disable the item while disconnecting to avoid repeated clicks
			app.disconnectItem.Disable()

			err := app.disconnectVPN()
			if err != nil {
				log.Printf("Error disconnecting: %v", err)
				// Re-enable so user can retry
				app.disconnectItem.Enable()
				continue
			}

			// Wait for autossh to actually exit and for the SOCKS port to close.
			// Poll several times over ~5 seconds.
			for i := 0; i < 20; i++ {
				if !app.isVPNConnected() {
					break
				}
				time.Sleep(250 * time.Millisecond)
			}

			log.Printf("Disconnected (or timed out waiting for process to exit)")
			app.updateIcon()
			// Leave the disconnect item disabled when disconnected
		}
	}()

	systray.AddSeparator()

	// Edit configuration
	editConfigItem := systray.AddMenuItem("⚙️ Edit Configuration", "Edit VPN configuration file")
	go func() {
		for {
			<-editConfigItem.ClickedCh
			err := app.editConfiguration()
			if err != nil {
				log.Printf("Error editing configuration: %v", err)
			}
		}
	}()

	systray.AddSeparator()

	// Quit
	quitItem := systray.AddMenuItem("Quit", "Quit the application")
	go func() {
		<-quitItem.ClickedCh
		systray.Quit()
	}()
}

func (app *VPNApp) updateIcon() {
	// Compute connection state once to avoid races and repeated expensive calls
	connected := app.isVPNConnected()

	// Prefer bundled image icons; otherwise use emoji titles
	usr, _ := user.Current()
	resourcesDir := filepath.Join(usr.HomeDir, "") // default empty; when bundled, resources will be bundled in the app package

	// Try to load icon from known relative path inside app bundle Resources
	exePath, err := os.Executable()
	var baseDir string
	if err == nil {
		baseDir = filepath.Dir(exePath)
	}

	// Check common Resources locations
	resourcesCandidates := []string{
		filepath.Join(baseDir, "../Resources"), // when running from Contents/MacOS
		filepath.Join(baseDir, "Resources"),    // when running from project dir
		filepath.Join("./Resources"),           // relative
		resourcesDir,                           // fallback to home (unlikely)
	}

	var iconSet bool
	for _, rdir := range resourcesCandidates {
		if rdir == "" {
			continue
		}
		var iconPath string
		if connected {
			iconPath = filepath.Join(rdir, "connected.png")
			tooltip := "SOCKS VPN Menu - Connected"
			if iconBytes, err := os.ReadFile(iconPath); err == nil {
				// Use image icon and clear title
				systray.SetIcon(iconBytes)
				systray.SetTitle("")
				systray.SetTooltip(tooltip)
				iconSet = true
				break
			}
			// fallback to emoji
			systray.SetTitle("🌎")
			systray.SetTooltip(tooltip)
			iconSet = true
			break
		} else {
			iconPath = filepath.Join(rdir, "disconnected.png")
			tooltip := "SOCKS VPN Menu - Disconnected"
			if iconBytes, err := os.ReadFile(iconPath); err == nil {
				systray.SetIcon(iconBytes)
				systray.SetTitle("")
				systray.SetTooltip(tooltip)
				iconSet = true
				break
			}
			// fallback to emoji
			systray.SetTitle("🌐")
			systray.SetTooltip(tooltip)
			iconSet = true
			break
		}
	}

	if !iconSet {
		// final fallback: emoji titles
		if connected {
			systray.SetTitle("🌎")
			systray.SetTooltip("SOCKS VPN Menu - Connected")
		} else {
			systray.SetTitle("🌐")
			systray.SetTooltip("SOCKS VPN Menu - Disconnected")
		}
	}

	// Sync disconnect menu state
	if app.disconnectItem != nil {
		if connected {
			app.disconnectItem.Enable()
		} else {
			app.disconnectItem.Disable()
		}
	}
}

func (app *VPNApp) loadConfig() (*Config, error) {
	usr, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("failed to get current user: %w", err)
	}

	configPath := filepath.Join(usr.HomeDir, ".vpn.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Set defaults if not specified
	if config.AutoSSHPath == "" {
		config.AutoSSHPath = "/opt/homebrew/bin/autossh"
	}
	if config.LocalPort == 0 {
		config.LocalPort = 1234
	}
	if config.Interface == "" {
		config.Interface = "Wi-Fi"
	}
	if config.ServerOptions.ServerAliveInterval == 0 {
		config.ServerOptions.ServerAliveInterval = 10
	}
	if config.ServerOptions.ServerAliveCountMax == 0 {
		config.ServerOptions.ServerAliveCountMax = 3
	}

	return &config, nil
}

func (app *VPNApp) isVPNConnected() bool {
	// Method 1: Check for autossh process with -D flag
	cmd := exec.Command("pgrep", "-f", "autossh.*-D.*localhost")
	output, err := cmd.Output()
	if err == nil && len(bytes.TrimSpace(output)) > 0 {
		log.Printf("VPN connected: autossh process detected (with -D)")
		return true
	}

	// Method 2: Fallback to any autossh process
	cmd2 := exec.Command("pgrep", "autossh")
	if err2 := cmd2.Run(); err2 == nil {
		log.Printf("VPN connected: autossh process detected (fallback)")
		return true
	}

	// Method 3: look for a TCP LISTEN on the configured local port from ssh/autossh
	if app.config != nil && app.config.LocalPort > 0 {
		// Use -nP and LISTEN to avoid TIME_WAIT/ESTABLISHED matches
		lsofCmd := exec.Command("lsof", "-nP", fmt.Sprintf("-iTCP:%d", app.config.LocalPort), "-sTCP:LISTEN")
		out3, err3 := lsofCmd.Output()
		if err3 == nil && len(bytes.TrimSpace(out3)) > 0 {
			// Inspect lines (skip header) for ssh/autossh in the COMMAND column
			text := strings.TrimSpace(string(out3))
			lines := strings.Split(text, "\n")
			if len(lines) > 1 {
				for _, ln := range lines[1:] {
					lower := strings.ToLower(ln)
					if strings.Contains(lower, "autossh") || strings.Contains(lower, "ssh") {
						log.Printf("VPN connected: lsof shows SSH listener: %s", strings.TrimSpace(ln))
						return true
					}
				}
			}
			// listener found but not ssh; ignore
			log.Printf("lsof reports listener on port %d but not SSH: %q", app.config.LocalPort, text)
			return false
		}
	}

	log.Printf("VPN disconnected: no autossh process or SSH listener on port %d", func() int {
		if app.config != nil {
			return app.config.LocalPort
		}
		return 0
	}())
	return false
}

func (app *VPNApp) connectVPN(commandName string) error {
	var targetCommand *Command
	for _, cmd := range app.config.Commands {
		if cmd.Name == commandName {
			targetCommand = &cmd
			break
		}
	}

	if targetCommand == nil {
		return fmt.Errorf("command '%s' not found in configuration", commandName)
	}

	// Disconnect any existing connection first
	app.disconnectVPN()

	// Build autossh command
	args := []string{
		"-f", "-M", "0",
		"-o", fmt.Sprintf("ServerAliveInterval %d", app.config.ServerOptions.ServerAliveInterval),
		"-o", fmt.Sprintf("ServerAliveCountMax %d", app.config.ServerOptions.ServerAliveCountMax),
		"-D", fmt.Sprintf("localhost:%d", app.config.LocalPort),
		"-N", targetCommand.Server,
	}

	cmd := exec.Command(app.config.AutoSSHPath, args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start autossh: %w", err)
	}

	// Enable SOCKS proxy via networksetup
	proxyCmd := exec.Command("networksetup", "-setsocksfirewallproxy", app.config.Interface, "127.0.0.1", fmt.Sprintf("%d", app.config.LocalPort))
	if err := proxyCmd.Run(); err != nil {
		return fmt.Errorf("failed to set SOCKS proxy: %w", err)
	}

	return nil
}

func (app *VPNApp) disconnectVPN() error {
	// Turn off SOCKS proxy
	proxyCmd := exec.Command("networksetup", "-setsocksfirewallproxystate", app.config.Interface, "off")
	if err := proxyCmd.Run(); err != nil {
		log.Printf("Warning: failed to disable SOCKS proxy: %v", err)
	}

	// Kill autossh processes
	killCmd := exec.Command("pkill", "autossh")
	if err := killCmd.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			if status, ok := exitError.Sys().(syscall.WaitStatus); ok && status.ExitStatus() == 1 {
				// No processes found - this is fine
				return nil
			}
			log.Printf("pkill returned error: %v", err)
		} else {
			log.Printf("pkill failed: %v", err)
		}
	}

	// If autossh persists after pkill, try SIGKILL
	checkCmd := exec.Command("pgrep", "autossh")
	if err := checkCmd.Run(); err == nil {
		// autossh still present; attempt SIGKILL
		log.Printf("autossh processes still present after pkill, issuing SIGKILL")
		kill9 := exec.Command("pkill", "-9", "autossh")
		if err := kill9.Run(); err != nil {
			log.Printf("SIGKILL attempt failed: %v", err)
			// Give up but don't return fatal error; the caller will poll and update status
		}
	}

	return nil
}

// editConfiguration opens ~/.vpn.json in an editor (creates a dummy if missing)
func (app *VPNApp) editConfiguration() error {
	usr, err := user.Current()
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}

	configPath := filepath.Join(usr.HomeDir, ".vpn.json")

	// Create a dummy configuration if it doesn't exist
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Printf("Configuration file doesn't exist, creating dummy configuration...")
		err := app.createDummyConfig(configPath)
		if err != nil {
			return fmt.Errorf("failed to create dummy configuration: %w", err)
		}
	}

	// Get the default editor or fall back to nano
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "nano"
	}

	// For macOS, we can also try to use 'open -e' for TextEdit or 'code' for VS Code
	// Let's try a few options in order of preference
	editors := []string{
		"code", // VS Code
		"subl", // Sublime Text
		"atom", // Atom
		"nano", // Nano (terminal-based)
		"vim",  // Vim (terminal-based)
	}

	var cmd *exec.Cmd
	var foundEditor string

	// Try to find an available editor
	for _, ed := range editors {
		if _, err := exec.LookPath(ed); err == nil {
			foundEditor = ed
			break
		}
	}

	if foundEditor == "" {
		// Fall back to macOS default text editor
		foundEditor = "open"
		cmd = exec.Command(foundEditor, "-e", configPath)
	} else if foundEditor == "open" {
		cmd = exec.Command(foundEditor, "-e", configPath)
	} else {
		// For terminal editors, we need to open a terminal
		if foundEditor == "nano" || foundEditor == "vim" {
			// Open in Terminal.app
			script := fmt.Sprintf("tell application \"Terminal\" to do script \"%s %s\"", foundEditor, configPath)
			cmd = exec.Command("osascript", "-e", script)
		} else {
			// For GUI editors
			cmd = exec.Command(foundEditor, configPath)
		}
	}

	log.Printf("Opening configuration file with %s...", foundEditor)

	// Start the editor
	err = cmd.Start()
	if err != nil {
		return fmt.Errorf("failed to open editor: %w", err)
	}

	// Wait for the editor to close (for terminal editors) or start a background monitor
	if foundEditor == "nano" || foundEditor == "vim" {
		// For terminal editors, wait for completion
		go func() {
			cmd.Wait()
			app.reloadConfiguration()
		}()
	} else {
		// For GUI editors, monitor file changes
		go app.monitorConfigFile(configPath)
	}

	return nil
}

// createDummyConfig creates a basic configuration file with example content
func (app *VPNApp) createDummyConfig(configPath string) error {
	dummyConfig := Config{
		AutoSSHPath: "/opt/homebrew/bin/autossh",
		LocalPort:   1234,
		Interface:   "Wi-Fi",
		ServerOptions: ServerOptions{
			ServerAliveInterval: 10,
			ServerAliveCountMax: 3,
		},
		Commands: []Command{
			{
				Name:        "example",
				Description: "Example VPN Server",
				Server:      "your-server-name-or-ip",
			},
		},
	}

	data, err := json.MarshalIndent(dummyConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal dummy config: %w", err)
	}

	err = os.WriteFile(configPath, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write dummy config: %w", err)
	}

	log.Printf("Created dummy configuration at %s", configPath)
	return nil
}

// monitorConfigFile watches for changes to the configuration file
func (app *VPNApp) monitorConfigFile(configPath string) {
	var lastModTime time.Time

	// Get initial modification time
	if stat, err := os.Stat(configPath); err == nil {
		lastModTime = stat.ModTime()
	}

	// Poll for changes every 2 seconds for 5 minutes
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	timeout := time.After(5 * time.Minute) // Stop monitoring after 5 minutes

	for {
		select {
		case <-ticker.C:
			if stat, err := os.Stat(configPath); err == nil {
				if stat.ModTime().After(lastModTime) {
					log.Printf("Configuration file changed, reloading...")
					app.reloadConfiguration()
					return // Stop monitoring after first change
				}
			}
		case <-timeout:
			log.Printf("Stopped monitoring configuration file (timeout)")
			return
		}
	}
}

// reloadConfiguration reloads the configuration from file
func (app *VPNApp) reloadConfiguration() {
	log.Printf("Reloading configuration...")

	config, err := app.loadConfig()
	if err != nil {
		log.Printf("Error reloading configuration: %v", err)
		return
	}

	app.config = config
	log.Printf("Configuration reloaded successfully with %d commands", len(config.Commands))

	// Note: With the current systray library, we cannot dynamically rebuild the menu
	// The user will need to restart the app to see new menu items
	// We could show a notification here if needed
}
