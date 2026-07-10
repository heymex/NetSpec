#!/usr/bin/env python3
import json
import logging
import logging.handlers
import os
import re
import socket
import time
from pathlib import Path
from typing import Dict, List, Optional, Tuple

SRC_FILE = Path(os.getenv("MDT_DECODED_FILE", "/sidecar/decoded.json"))
DEST_HOST = os.getenv("NETSPEC_INGEST_HOST", "127.0.0.1")
DEST_PORT = int(os.getenv("NETSPEC_INGEST_PORT", "57500"))
DEST_TARGETS = os.getenv("NETSPEC_INGEST_TARGETS", "").strip()
LOG_FILE = Path(os.getenv("MDT_FORWARDER_LOG", "/sidecar/forwarder.log"))
LOG_MAX_BYTES = int(os.getenv("MDT_FORWARDER_LOG_MAX_BYTES", str(10 * 1024 * 1024)))
LOG_BACKUP_COUNT = int(os.getenv("MDT_FORWARDER_LOG_BACKUP_COUNT", "3"))
RESEND_INTERVAL_SECONDS = int(os.getenv("MDT_RESEND_INTERVAL_SECONDS", "60"))
DECODED_WARN_BYTES = int(os.getenv("MDT_DECODED_WARN_BYTES", str(100 * 1024 * 1024)))
PRUNE_DECODED_ARCHIVES_ON_START = os.getenv("MDT_PRUNE_DECODED_ARCHIVES_ON_START", "true").strip().lower() in (
    "1",
    "true",
    "yes",
    "on",
)

allowed_env = os.getenv("MDT_ALLOWED_DEVICES", "").strip()
ALLOWED_DEVICES = {d.strip() for d in allowed_env.split(",") if d.strip()} if allowed_env else set()

ARCHIVE_SUFFIX_RE = re.compile(r"^decoded\.json\.\d+$")


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


def setup_logging() -> logging.Logger:
    LOG_FILE.parent.mkdir(parents=True, exist_ok=True)
    logger = logging.getLogger("mdt-forwarder")
    logger.setLevel(logging.INFO)
    logger.handlers.clear()
    handler = logging.handlers.RotatingFileHandler(
        LOG_FILE,
        maxBytes=LOG_MAX_BYTES,
        backupCount=LOG_BACKUP_COUNT,
        encoding="utf-8",
    )
    handler.setFormatter(logging.Formatter("%(asctime)s %(message)s", datefmt="%Y-%m-%d %H:%M:%S"))
    logger.addHandler(handler)
    return logger


def format_bytes(num_bytes: int) -> str:
    if num_bytes < 1024:
        return f"{num_bytes}B"
    if num_bytes < 1024 * 1024:
        return f"{num_bytes / 1024:.1f}KB"
    if num_bytes < 1024 * 1024 * 1024:
        return f"{num_bytes / (1024 * 1024):.1f}MB"
    return f"{num_bytes / (1024 * 1024 * 1024):.2f}GB"


def prune_decoded_archives(sidecar_dir: Path, logger: logging.Logger) -> int:
    """Remove Telegraf rotation archives; the forwarder only tails the active file."""
    removed = 0
    for path in sorted(sidecar_dir.glob("decoded.json.*")):
        if not ARCHIVE_SUFFIX_RE.match(path.name):
            continue
        try:
            size = path.stat().st_size
            path.unlink()
            removed += 1
            logger.info("pruned_archive path=%s size=%s", path.name, format_bytes(size))
        except OSError as exc:
            logger.warning("prune_archive_failed path=%s error=%s", path, exc)
    return removed


class DecodedTail:
    """Tail -F style reader for Telegraf's decoded.json (handles rotation/truncation)."""

    def __init__(self, path: Path):
        self.path = path
        self._file: Optional[object] = None
        self._inode: Optional[int] = None
        self._open_tail()

    def _open_tail(self, *, from_start: bool = False):
        if self._file is not None:
            self._file.close()
            self._file = None
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self.path.touch(exist_ok=True)
        self._file = self.path.open("r", encoding="utf-8")
        st = os.fstat(self._file.fileno())
        self._inode = st.st_ino
        if from_start:
            self._file.seek(0)
        else:
            self._file.seek(0, 2)

    def _maybe_reopen(self):
        try:
            st_path = self.path.stat()
        except FileNotFoundError:
            self._open_tail(from_start=True)
            return
        if self._file is None:
            self._open_tail()
            return
        if st_path.st_ino != self._inode:
            self._open_tail(from_start=True)
            return
        pos = self._file.tell()
        if st_path.st_size < pos:
            self._open_tail(from_start=True)

    def readline(self) -> str:
        if self._file is None:
            self._open_tail()
        line = self._file.readline()
        if line:
            return line
        self._maybe_reopen()
        if self._file is None:
            return ""
        return self._file.readline()

    def close(self):
        if self._file is not None:
            self._file.close()
            self._file = None


def main():
    logger = setup_logging()
    logger.info("forwarder starting")
    SRC_FILE.parent.mkdir(parents=True, exist_ok=True)
    SRC_FILE.touch(exist_ok=True)

    if PRUNE_DECODED_ARCHIVES_ON_START:
        pruned = prune_decoded_archives(SRC_FILE.parent, logger)
        if pruned:
            logger.info("pruned_decoded_archives count=%d", pruned)

    try:
        active_size = SRC_FILE.stat().st_size
    except OSError:
        active_size = 0
    if active_size >= DECODED_WARN_BYTES:
        logger.warning(
            "decoded_json_large size=%s note=tail-only; configure Telegraf rotation_max_size "
            "or stop telegraf and truncate %s if disk is critical",
            format_bytes(active_size),
            SRC_FILE,
        )

    targets = parse_targets()
    logger.info("targets=%s", ",".join([f"{host}:{port}" for host, port in targets]))
    socks: Dict[Tuple[str, int], socket.socket] = {
        target: connect_sock(target[0], target[1]) for target in targets
    }
    sent = 0
    last_state = {}
    last_sent_at = {}

    tail = DecodedTail(SRC_FILE)
    try:
        while True:
            line = tail.readline()
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
                    logger.info("sent=%d", sent)
            except Exception as exc:
                logger.warning("parse_error=%s", exc)
    finally:
        tail.close()


if __name__ == "__main__":
    main()
