import asyncio
import json
import os
import sys
import tempfile
import time
from typing import Any, Dict, List, Tuple

import aiohttp
from aiohttp_socks import ProxyConnector

from core.config import Settings

class XrayService:
    def __init__(self, settings: Settings):
        self.settings = settings

    def build_xray_config_for_batch(self, servers: List[Dict[str, Any]], base_port: int) -> Dict[str, Any]:
        inbounds, outbounds, routing_rules = [], [], []
        for i, server in enumerate(servers):
            inbound_port = base_port + i
            inbound_tag, outbound_tag = f"in-{i}", f"out-{i}"
            inbounds.append({
                "tag": inbound_tag, "port": inbound_port, "listen": "127.0.0.1", "protocol": "socks",
                "settings": {"auth": "noauth", "udp": True, "ip": "127.0.0.1"},
            })

            protocol = server.get("protocol")
            outbound_config = None

            if protocol == "vless":
                vnext = [{"address": server["address"], "port": server["port"], "users": [{
                    "id": server["vless_id"], "encryption": "none", "flow": server.get("flow", "")
                }]}]
                stream_settings = {"network": server.get("type", "tcp"), "security": server.get("security", "none")}
                # Sanitize security field
                if stream_settings["security"] == "auto":
                    stream_settings["security"] = "none"

                if stream_settings["network"] == "ws":
                    stream_settings["wsSettings"] = {"path": server.get("path", "/")}
                    ws_host = server.get("host", server["address"])
                    if ws_host:
                        stream_settings["wsSettings"]["host"] = ws_host
                
                if stream_settings["security"] in ("tls", "reality"):
                    security_settings = {"serverName": server.get("sni", server.get("host", server["address"])), "fingerprint": server.get("fp", "chrome")}
                    if stream_settings["security"] == "reality":
                        security_settings.update({"publicKey": server.get("pbk"), "shortId": server.get("sid")})
                    setting_key = f"{stream_settings['security']}Settings"
                    stream_settings[setting_key] = security_settings
                outbound_config = {"protocol": "vless", "settings": {"vnext": vnext}, "streamSettings": stream_settings}
            elif protocol == "vmess":
                vnext = [{"address": server["address"], "port": server["port"], "users": [{
                    "id": server["vmess_id"], "alterId": server.get("aid", 0), "security": server.get("security", "auto")
                }]}]
                stream_settings = {"network": server.get("type", "tcp"), "security": server.get("tls", "none")}
                # Sanitize security field
                if stream_settings["security"] == "auto":
                    stream_settings["security"] = "none"

                if stream_settings["network"] == "ws":
                    stream_settings["wsSettings"] = {"path": server.get("path", "/")}
                    ws_host = server.get("host", server["address"])
                    if ws_host:
                        stream_settings["wsSettings"]["host"] = ws_host
                
                if stream_settings["security"] == "tls":
                        stream_settings["tlsSettings"] = {"serverName": server.get("sni", server.get("host", server["address"]))}
                outbound_config = {"protocol": "vmess", "settings": {"vnext": vnext}, "streamSettings": stream_settings}
            elif protocol == "trojan":
                server_config = [{"address": server["address"], "port": server["port"], "password": server["password"]}]
                stream_settings = {"network": server.get("type", "tcp"), "security": "tls"}
                stream_settings["tlsSettings"] = {"serverName": server.get("sni", server.get("host", server["address"]))}
                if server.get("type") == "ws":
                    stream_settings["wsSettings"] = {"path": server.get("path", "/")}
                    ws_host = server.get("host", server["address"])
                    if ws_host:
                        stream_settings["wsSettings"]["host"] = ws_host
                outbound_config = {"protocol": "trojan", "settings": {"servers": server_config}, "streamSettings": stream_settings}
            elif protocol == "shadowsocks":
                server_config = [{"address": server["address"], "port": server["port"], "method": server["method"], "password": server["password"]}]
                outbound_config = {"protocol": "shadowsocks", "settings": {"servers": server_config}}
            elif protocol == "hysteria2":
                server_info = {
                    "address": server["address"],
                    "port": server["port"],
                    "password": server["password"]
                }
                if server.get("obfs") and server["obfs"] != "none":
                     server_info["obfs"] = {
                        "type": server["obfs"],
                        "password": server.get("obfs_password", "")
                    }
                
                stream_settings = {
                    "security": "tls",
                    "tlsSettings": {
                        "serverName": server.get("sni", server.get("host", server["address"])),
                        "allowInsecure": server.get("insecure", False)
                    }
                }
                outbound_config = {
                    "protocol": "hysteria2",
                    "settings": {"servers": [server_info]},
                    "streamSettings": stream_settings
                }

            if outbound_config:
                outbound_config["tag"] = outbound_tag
                outbounds.append(outbound_config)
                routing_rules.append({"type": "field", "inboundTag": [inbound_tag], "outboundTag": outbound_tag})

        return {"log": {"loglevel": "warning"}, "inbounds": inbounds, "outbounds": outbounds, "routing": {"rules": routing_rules}}


    async def _wait_for_ports(self, ports: List[int], timeout: float = 5.0) -> bool:
        """Waits until the first port is listening, indicating Xray has started."""
        if not ports: return True
        
        start_time = time.time()
        port = ports[0]
        while time.time() - start_time < timeout:
            try:
                # Try to connect to the port
                reader, writer = await asyncio.open_connection('127.0.0.1', port)
                writer.close()
                await writer.wait_closed()
                return True
            except (ConnectionRefusedError, OSError):
                await asyncio.sleep(0.1)
        return False

    async def test_server_real_delay(self, port: int) -> float:
        """Tests latency by making a request through the local SOCKS5 proxy."""
        proxy_url = f"socks5://127.0.0.1:{port}"
        try:
            connector = ProxyConnector.from_url(proxy_url)
            async with aiohttp.ClientSession(connector=connector) as proxy_session:
                start_time = time.monotonic()
                async with proxy_session.head(self.settings.LATENCY_TEST_URL, timeout=self.settings.TEST_TIMEOUT) as response:
                    if 200 <= response.status < 300:
                        return (time.monotonic() - start_time) * 1000
                    return float("inf")
        except Exception:
            return float("inf")


    async def check_url_via_proxy(self, port: int, target_url: str) -> bool:
        """Checks if a target URL is accessible via a SOCKS5 proxy, returning True on success."""
        proxy = f"socks5://127.0.0.1:{port}"
        try:
            connector = ProxyConnector.from_url(proxy)
            timeout = aiohttp.ClientTimeout(total=self.settings.TEST_TIMEOUT)
            async with aiohttp.ClientSession(connector=connector, timeout=timeout) as session:
                async with session.head(target_url, allow_redirects=True, timeout=self.settings.TEST_TIMEOUT) as response:
                    return response.status < 400
        except Exception:
            return False


    async def run_test_batch(self, servers: List[Dict[str, Any]]) -> List[Tuple[Dict, float]]:
        if not servers:
            return []

        xray_config = self.build_xray_config_for_batch(servers, self.settings.BASE_PORT)
        tmp = tempfile.NamedTemporaryFile(mode="w+", delete=False, suffix=".json", encoding="utf-8")
        await asyncio.to_thread(json.dump, xray_config, tmp)
        await asyncio.to_thread(tmp.close)
        config_path = tmp.name

        process = None
        try:
            env = os.environ.copy()
            if os.path.isdir(self.settings.XRAY_ASSETS_PATH):
                env["XRAY_LOCATION_ASSET"] = self.settings.XRAY_ASSETS_PATH

            process = await asyncio.create_subprocess_exec(
                self.settings.XRAY_PATH, "-c", config_path,
                stdout=asyncio.subprocess.PIPE, stderr=asyncio.subprocess.PIPE, env=env
            )
            
            # Smart polling: wait for Xray ports to be ready
            ports = [self.settings.BASE_PORT + i for i in range(len(servers))]
            await self._wait_for_ports(ports, timeout=3.0)

            if process.returncode is not None:
                stdout_data = await process.stdout.read()
                stderr_data = await process.stderr.read()
                print(f"Xray process failed to start.", file=sys.stderr)
                print(f"Stdout: {stdout_data.decode()}", file=sys.stderr)
                print(f"Stderr: {stderr_data.decode()}", file=sys.stderr)
                return [(s, float("inf")) for s in servers]

            tasks = [self.test_server_real_delay(self.settings.BASE_PORT + i) for i, _ in enumerate(servers)]
            results = await asyncio.gather(*tasks)
            return list(zip(servers, results))

        except FileNotFoundError:
            print(f"Error: Xray not found at '{self.settings.XRAY_PATH}'.", file=sys.stderr)
            return [(s, float("inf")) for s in servers]
        except Exception as e:
            print(f"An error occurred during batch testing: {e}", file=sys.stderr)
            return [(s, float("inf")) for s in servers]
        finally:
            if process and process.returncode is None:
                try:
                    process.terminate()
                    await asyncio.wait_for(process.wait(), timeout=2)
                except asyncio.TimeoutError:
                    process.kill()
                await process.wait()
            if os.path.exists(config_path):
                await asyncio.to_thread(os.remove, config_path)
    
    async def evaluate_site_accessibility(self, url: str, servers_to_test: List[Dict]) -> List[Dict]:
        """Helper to test a list of servers against a specific URL."""
        successful_servers = []
        
        for i in range(0, len(servers_to_test), self.settings.BATCH_SIZE):
            batch = servers_to_test[i : i + self.settings.BATCH_SIZE]
            print(f"Testing batch {i // self.settings.BATCH_SIZE + 1} for site: {url}")

            xray_config = self.build_xray_config_for_batch(batch, self.settings.BASE_PORT)
            tmp = tempfile.NamedTemporaryFile(mode="w+", delete=False, suffix=".json", encoding="utf-8")
            await asyncio.to_thread(json.dump, xray_config, tmp)
            await asyncio.to_thread(tmp.close)
            config_path = tmp.name

            process = None
            try:
                env = os.environ.copy()
                if os.path.isdir(self.settings.XRAY_ASSETS_PATH):
                    env["XRAY_LOCATION_ASSET"] = self.settings.XRAY_ASSETS_PATH

                process = await asyncio.create_subprocess_exec(
                    self.settings.XRAY_PATH, "-c", config_path,
                    stdout=asyncio.subprocess.PIPE, stderr=asyncio.subprocess.PIPE, env=env
                )
                
                # Smart polling: wait for Xray ports to be ready
                ports = [self.settings.BASE_PORT + j for j in range(len(batch))]
                await self._wait_for_ports(ports, timeout=3.0)

                if process.returncode is not None:
                    stdout_data = await process.stdout.read()
                    stderr_data = await process.stderr.read()
                    print(f"Xray process (site check) failed to start.", file=sys.stderr)
                    print(f"Stdout: {stdout_data.decode()}", file=sys.stderr)
                    print(f"Stderr: {stderr_data.decode()}", file=sys.stderr)
                    continue

                tasks = [self.check_url_via_proxy(self.settings.BASE_PORT + j, url) for j, _ in enumerate(batch)]
                results = await asyncio.gather(*tasks)

                for server, was_successful in zip(batch, results):
                    if was_successful:
                        successful_servers.append(server)

            finally:
                if process and process.returncode is None:
                    try:
                        process.terminate()
                        await asyncio.wait_for(process.wait(), timeout=2)
                    except asyncio.TimeoutError:
                        process.kill()
                    await process.wait()
                if os.path.exists(config_path):
                    await asyncio.to_thread(os.remove, config_path)
        
        return successful_servers
