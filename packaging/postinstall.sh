#!/bin/sh
set -e

warn() {
    echo "li-gh-proxy: $1"
}

if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload || warn "systemd reload failed"
    systemctl enable li-gh-proxy >/dev/null 2>&1 || warn "systemd enable failed"

    if [ -d /run/systemd/system ]; then
        systemctl restart li-gh-proxy || systemctl start li-gh-proxy || {
            warn "service start failed, check: journalctl -u li-gh-proxy"
        }
    fi
fi

if command -v rc-update >/dev/null 2>&1; then
    rc-update add li-gh-proxy default >/dev/null 2>&1 || warn "OpenRC enable failed"
fi

if command -v rc-service >/dev/null 2>&1; then
    rc-service li-gh-proxy restart || rc-service li-gh-proxy start || {
        warn "service start failed, check: rc-service li-gh-proxy status"
    }
fi
