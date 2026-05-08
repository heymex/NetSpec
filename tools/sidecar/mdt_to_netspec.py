#!/usr/bin/env python3
import json
import os
import socket
import time
from pathlib import Path
from typing import Dict, List, Tuple

SRC_FILE = Path(os.getenv("MDT_DECODED_FILE", "/sidecar/decoded.json"))
DEST_HOST = os.getenv("NETSPEC_INGEST_HOST", "127.0.0.1")
DEST_PORT = int(os.getenv("NETSPEC_INGEST_PORT", "57500"))
DEST_TARGETS = os.getenv("NETSPEC_INGEST_TARGETS", "").strip()
LOG_FILE = Path(os.getenv("MDT_FORWARDER_LOG", "/sidecar/forwarder.log"))
RESEND_INTERVAL_SECONDS = int(os.getenv("MDT_RESEND_INTERVAL_SECONDS", "60"))

allowed_env = os.getenv("MDT_ALLOWED_DEVICES", "").strip()
ALLOWED_DEVICES = {d.strip() for d in allowed_env.split(",") if d.strip()} if allowed_env else set()


def parse_targets() -> List[Tuple[str, int]]:
    targets: List[Tuple[str, int]] = []
    if DEST_TARGETS:
        for raw_entry in DEST_TARGETS.split(","):
            entry = raw_entry.strip()
            if not entry:
                continue
            if ":" not in entry:
                raise ValueError(
                    f"invalid target '{entry}' in NETSPEC_INGEST_TARGETS; expected host:port"
                )
            host, port_s = entry.rsplit(":", 1)
            host = host.strip()
            port_s = port_s.strip()
            if not host:
                raise ValueError(
                    f"invalid target '{entry}' in NETSPEC_INGEST_TARGETS; host cannot be empty"
                )
            try:
                port = int(port_s)
            except ValueError as exc:
                raise ValueError(
                    f"invalid target '{entry}' in NETSPEC_INGEST_TARGETS; port must be an integer"
                ) from exc
            if port <= 0 or port > 65535:
                raise ValueError(
                    f"invalid target '{entry}' in NETSPEC_INGEST_TARGETS; port out of range"
                )
            targets.append((host, port))
    else:
        targets.append((DEST_HOST, DEST_PORT))

    deduped: List[Tuple[str, int]] = []
    seen = set()
    for target in targets:
        if target in seen:
            continue
        seen.add(target)
        deduped.append(target)
    return deduped


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


def connect_sock(host: str, port: int):
    while True:
        try:
            sock = socket.create_connection((host, port), timeout=5)
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

    targets = parse_targets()
    log(f"targets={','.join([f'{host}:{port}' for host, port in targets])}")
    socks: Dict[Tuple[str, int], socket.socket] = {
        target: connect_sock(target[0], target[1]) for target in targets
    }
    sent = 0
    last_state = {}
    last_sent_at = {}

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
                now = time.time()
                if (
                    last_state.get(key) == state_key
                    and (now - last_sent_at.get(key, 0)) < RESEND_INTERVAL_SECONDS
                ):
                    continue
                last_state[key] = state_key
                last_sent_at[key] = now

                payload = (json.dumps(event) + "\n").encode("utf-8")
                for target in targets:
                    sock = socks[target]
                    try:
                        sock.sendall(payload)
                    except Exception:
                        try:
                            sock.close()
                        except Exception:
                            pass
                        sock = connect_sock(target[0], target[1])
                        socks[target] = sock
                        sock.sendall(payload)

                sent += 1
                if sent % 50 == 0:
                    log(f"sent={sent}")
            except Exception as exc:
                log(f"parse_error={exc}")


if __name__ == "__main__":
    main()
