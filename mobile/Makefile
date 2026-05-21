# Vior Mobile — Gio app build targets
# Requires: go install gioui.org/cmd/gogio@latest

VERSION = 0.1.0.1

.PHONY: run android ios

# Run on desktop (for testing)
run:
	go run .

# Build Android APK
android:
	gogio -target android -appid com.vior.mobile -version $(VERSION) -o vior-mobile.apk .

# Build iOS (requires macOS + Xcode)
ios:
	gogio -target ios -appid com.vior.mobile -version $(VERSION) .

# Install gogio build tool
install-tools:
	go install gioui.org/cmd/gogio@latest
