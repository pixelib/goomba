# Create the local dir in your goomba source tree
mkdir -p embeddedsk/apple-sdk-lite
echo "present" > embeddedsk/sdk_marker.txt

# Get your current SDK path
SDK_PATH=$(xcrun --sdk macosx --show-sdk-path)

# Copy ONLY headers and .tbd files (the "stubs")
# This reduces the size from ~1GB to ~30MB
rsync -Rrav --include="*/" --include="*.h" --include="*.tbd" --exclude="*" "$SDK_PATH/" internal/embeddedsk/apple-sdk-lite/