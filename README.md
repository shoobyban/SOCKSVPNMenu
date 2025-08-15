# SOCKS VPN Menu Bar Application

A native macOS menu bar application for managing SOCKS proxy VPN connections using autossh.

## Build, Installation & Setup

**Install dependencies**:
   * autossh (through Homebrew: `brew install autossh`)
   * go (1.23 or higher, through Homebrew: `brew install go`)
   * good ~/.ssh/config without password authentication
   * server(s) where dynamic port forwarding is enabled (can't help you with this :))

**Build the application**:
   ```bash
   ./build-app-bundle.sh
   ```

**Copy the example configuration**:
   ```bash
   cp ../example-vpn.json ~/.vpn.json
   ```

**Edit your VPN configuration**:
   ```bash
   vi ~/.vpn.json
   ```

   Or just use the menu item "Edit VPN Configuration" if you have editor, it will open.

**Run the application**:
   ```bash
   open SOCKS\ VPN\ Menu.app
   ```
  Or copy it to `/Applications` and run it from there.

## License

MIT
