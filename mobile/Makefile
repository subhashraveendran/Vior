# Vior Mobile — Gio app build targets
# Requires: go install gioui.org/cmd/gogio@latest

PERMISSIONS = android.permission.INTERNET,android.permission.ACCESS_NETWORK_STATE,android.permission.ACCESS_WIFI_STATE
VERSION = 0.1.0.1

.PHONY: run android ios

# Run on desktop (for testing)
run:
	go run .

# Build Android APK
android:
	gogio -target android -appid com.vior.mobile -version $(VERSION) -permissions $(PERMISSIONS) -o vior-mobile.apk .

# Build iOS (requires macOS + Xcode)
ios:
	gogio -target ios -appid com.vior.mobile -version $(VERSION) .

# Install gogio build tool
install-tools:
	go install gioui.org/cmd/gogio@latest
