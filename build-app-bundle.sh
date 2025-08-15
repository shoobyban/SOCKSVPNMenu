#!/bin/bash

set -e

APP_NAME="SOCKS VPN Menu"
BUNDLE_NAME="SOCKS VPN Menu.app"
EXECUTABLE_NAME="socks-vpn-menu"
BUNDLE_ID="com.socksvpn.menu"
VERSION="1.0.0"

echo "Building macOS app bundle: $BUNDLE_NAME"

rm -rf "$BUNDLE_NAME"

echo "Building Go binary..."
go build -ldflags="-s -w" -o "$EXECUTABLE_NAME" main.go

echo "Creating app bundle structure..."
mkdir -p "$BUNDLE_NAME/Contents/MacOS"
mkdir -p "$BUNDLE_NAME/Contents/Resources"

mv "$EXECUTABLE_NAME" "$BUNDLE_NAME/Contents/MacOS/"

echo "Creating Info.plist..."
cat > "$BUNDLE_NAME/Contents/Info.plist" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleExecutable</key>
    <string>$EXECUTABLE_NAME</string>
    <key>CFBundleIdentifier</key>
    <string>$BUNDLE_ID</string>
    <key>CFBundleName</key>
    <string>$APP_NAME</string>
    <key>CFBundleDisplayName</key>
    <string>$APP_NAME</string>
    <key>CFBundleVersion</key>
    <string>$VERSION</string>
    <key>CFBundleShortVersionString</key>
    <string>$VERSION</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleSignature</key>
    <string>????</string>
    <key>LSMinimumSystemVersion</key>
    <string>10.15</string>
    <key>LSUIElement</key>
    <true/>
    <key>NSHighResolutionCapable</key>
    <true/>
    <key>LSBackgroundOnly</key>
    <false/>
    <key>LSApplicationCategoryType</key>
    <string>public.app-category.utilities</string>
</dict>
</plist>
EOF

echo "Adding example configuration..."
cp example-vpn.json "$BUNDLE_NAME/Contents/Resources/"

echo "Creating simple icon..."
cat > "$BUNDLE_NAME/Contents/Resources/icon.txt" << EOF
This app bundle contains the SOCKS VPN Menu application.
EOF

chmod +x "$BUNDLE_NAME/Contents/MacOS/$EXECUTABLE_NAME"

cat > "$BUNDLE_NAME/Contents/MacOS/setup-helper.sh" << 'EOF'
#!/bin/bash
# Helper script to set up the VPN configuration

RESOURCES_DIR="$(dirname "$0")/../Resources"
CONFIG_FILE="$HOME/.vpn.json"

if [ ! -f "$CONFIG_FILE" ]; then
    echo "Setting up SOCKS VPN Menu..."
    echo "Copying example configuration to ~/.vpn.json"
    cp "$RESOURCES_DIR/example-vpn.json" "$CONFIG_FILE"
    echo "Please edit ~/.vpn.json with your VPN server details"
    echo "Then launch the app again"
    exit 0
fi
EOF

chmod +x "$BUNDLE_NAME/Contents/MacOS/setup-helper.sh"

echo "App bundle created successfully: $BUNDLE_NAME"
echo ""
echo "Note: For distribution, you need to code sign the app:"
echo "  codesign --deep --force --verify --verbose --sign 'Developer ID Application: Your Name' '$BUNDLE_NAME'"
