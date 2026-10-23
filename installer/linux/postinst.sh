#!/bin/sh
set -e
# Same script as `curl -fsSL https://octelium.com/install.sh | bash`, made safe
# to run inside dpkg/rpm: no nested apt/dnf (it would deadlock on the package
# lock), no sudo (we are root) and no official desktop app (this package is the GUI).
curl -fsSL https://octelium.com/install.sh |
	USE_PACKAGE_MANAGER=false USE_SUDO=false INSTALL_DESKTOP=false bash

systemctl daemon-reload
systemctl enable octelium-daemon.service
systemctl restart octelium-daemon.service
