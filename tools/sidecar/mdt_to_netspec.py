#!/usr/bin/env python3
import json
import os
import socket
import time
from pathlib import Path

SRC_FILE = Path(os.getenv("MDT_DECODED_FILE", "/sidecar/decoded.json"))
DEST_HOST = os.getenv("NETSPEC_INGEST_HOST", "127.0.0.1")
DEST_PORT = int(os.getenv("NETSPEC_INGEST_PORT", "57500"))
LOG_FILE = Path(os.getenv("MDT_FORWARDER_LOG", "/sidecar/forwarder.log"))

allowed_env = os.getenv("MDT_ALLOWED_DEVICES", "").strip()
ALLOWED_DEVICES = {d.strip() for d in allowed_env.split(",") if d.strip()} if allowed_env else set()


def norm_oper(value: str) -> str:
    if not value:
        return ""
    s = str(value).strip().lower()
    if s == "up":
        return "up"
    if s in ("down", "lower-layer-down", "dormant", "not-present"):
        return "down"
    return s


def norm_admin(value: str) -> str:
    if not value:
        return ""
    s = str(value).strip().lower()
    if s in ("up", "enabled"):
        return "up"
    if s in ("down", "disabled", "administratively-down"):
        return "down"
    return s


def connect_sock():
    while True:
        try:
            sock = socket.create_connection((DEST_HOST, DEST_PORT), timeout=5)
            sock.settimeout(5)
            return sock
        except Exception:
            time.sleep(1)


def log(msg: str):
    LOG_FILE.parent.mkdir(parents=True, exist_ok=True)
    with LOG_FILE.open("a", encoding="utf-8") as f:
        f.write(f"{time.strftime('%Y-%m-%d %H:%M:%S')} {msg}\n")


def main():
    log("forwarder starting")
    SRC_FILE.parent.mkdir(parents=True, exist_ok=True)
    SRC_FILE.touch(exist_ok=True)

    sock = connect_sock()
    sent = 0
    last_state = {}

    with SRC_FILE.open("r", encoding="utf-8") as f:
        f.seek(0, 2)
        while True:
            line = f.readline()
            if not line:
                time.sleep(0.2)
                continue
            line = line.strip()
            if not line:
                continue

            try:
                obj = json.loads(line)
                tags = obj.get("tags", {})
                fields = obj.get("fields", {})
                device = tags.get("source", "")
                iface = tags.get("name", "")
                oper = norm_oper(fields.get("oper_status", ""))
                admin = norm_admin(fields.get("admin_status", ""))

                if not device or not iface or (not oper and not admin):
                    continue
                if ALLOWED_DEVICES and device not in ALLOWED_DEVICES:
                    continue

                event = {
                    "device": device,
                    "interface": iface,
                    "oper_status": oper,
                    "admin_status": admin,
                    "source": "mdt-sidecar",
                }
                key = (device, iface)
                state_key = (oper, admin)
                if last_state.get(key) == state_key:
                    continue
                last_state[key] = state_key

                payload = (json.dumps(event) + "\n").encode("utf-8")
                try:
                    sock.sendall(payload)
                except Exception:
                    try:
                        sock.close()
                    except Exception:
                        pass
                    sock = connect_sock()
                    sock.sendall(payload)

                sent += 1
                if sent % 50 == 0:
                    log(f"sent={sent}")
            except Exception as exc:
                log(f"parse_error={exc}")


if __name__ == "__main__":
    main()
