#!/bin/bash
set -e

# 1. Setup paths
# Get the absolute path of the directory where this script is located
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
# We want the zip to end up in internal/embeddedsk/
TARGET_DIR="$SCRIPT_DIR/embeddedsk"
ZIP_NAME="sdk.zip"
MARKER_FILE="sdk_marker.txt"

# 2. Find the SDK path
SDK_PATH=$(xcrun --sdk macosx --show-sdk-path)
echo ">> Using SDK at: $SDK_PATH"

# 3. Create staging area
TEMP_STAGING=$(mktemp -d)
echo ">> Created staging area at: $TEMP_STAGING"

# Ensure cleanup on exit
trap 'rm -rf "$TEMP_STAGING"' EXIT

echo ">> Extracting ALL headers and stubs..."
# -R preserves the relative path from the root
rsync -Rrav \
    --include="*/" \
    --include="*.h" \
    --include="*.tbd" \
    --exclude="PrivateHeaders" \
    --exclude="Modules" \
    --exclude="*" \
    "$SDK_PATH/usr" \
    "$SDK_PATH/System/Library/Frameworks" \
    "$TEMP_STAGING/"

# 4. Zip it up
mkdir -p "$TARGET_DIR"

# Move into the staging directory's SDK root so the zip starts at /usr and /System
# This is the important part: we need to go deep into the temp folder structure
STAGED_SDK_ROOT="$TEMP_STAGING$SDK_PATH"

echo ">> Zipping SDK content..."
(
    cd "$STAGED_SDK_ROOT"
    zip -r -9 "$TARGET_DIR/$ZIP_NAME" .
)

# 5. Create marker
echo "present" > "$TARGET_DIR/$MARKER_FILE"

SIZE=$(du -sh "$TARGET_DIR/$ZIP_NAME" | cut -f1)
echo ">> Done!"
echo ">> ZIP Size: $SIZE"
echo ">> Saved to: $TARGET_DIR/$ZIP_NAME"