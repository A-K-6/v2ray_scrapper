import asyncio
import json
import os
import sys
import tempfile
import time
from typing import Any, Dict, List, Tuple, Optional

import aiohttp
from loguru import logger

from core.config import Settings
from models.server import ProxyServer

class XrayProcessContext:
    def __init__(self, settings: Settings, config: Dict[str, Any], ports: List[int], timeout: float = 10.0):
        self.settings = settings
        self.config = config
        self.ports = ports
        self.timeout = timeout
        self.process = None
        self.config_path = None
        self._tmp_file = None

    async def __aenter__(self):
        # 1. Write config to temp file
        self._tmp_file = tempfile.NamedTemporaryFile(mode="w+", delete=False, suffix=".json", encoding="utf-8")
        await asyncio.to_thread(json.dump, self.config, self._tmp_file)
        await asyncio.to_thread(self._tmp_file.close)
        self.config_path = self._tmp_file.name

        # 2. Start Xray Process
        env = os.environ.copy()
        if os.path.isdir(self.settings.XRAY_ASSETS_PATH):
            env["XRAY_LOCATION_ASSET"] = self.settings.XRAY_ASSETS_PATH

        try:
            self.process = await asyncio.create_subprocess_exec(
                self.settings.XRAY_PATH, "-c", self.config_path,
                stdout=asyncio.subprocess.PIPE, stderr=asyncio.subprocess.PIPE, env=env
            )
        except FileNotFoundError:
            logger.error(f"Error: Xray not found at '{self.settings.XRAY_PATH}'.")
            raise

        # 3. Wait for ports
        if not await self._wait_for_ports(self.ports, self.timeout):
            logger.error(f"Xray timed out waiting for ports to open (timeout={self.timeout}s).")
            # Capture output for debugging
            if self.process:
                try:
                    stdout, stderr = await asyncio.wait_for(self.process.communicate(), timeout=1.0)
                    logger.error(f"Stdout: {stdout.decode()}")
                    logger.error(f"Stderr: {stderr.decode()}")
                except asyncio.TimeoutError:
                    pass
            raise TimeoutError("Xray failed to bind ports in time.")

        # 4. Check for early exit
        if self.process.returncode is not None:
            stdout, stderr = await self.process.communicate()
            logger.error(f"Xray process failed to start immediately.")
            logger.error(f"Stdout: {stdout.decode()}")
            logger.error(f"Stderr: {stderr.decode()}")
            raise RuntimeError("Xray process crashed on startup.")

        return self.process

    async def __aexit__(self, exc_type, exc, tb):
        # Cleanup Process
        if self.process and self.process.returncode is None:
            try:
                self.process.terminate()
                await asyncio.wait_for(self.process.wait(), timeout=2.0)
            except asyncio.TimeoutError:
                logger.warning("Xray process did not terminate gracefully, killing...")
                self.process.kill()
                await self.process.wait()
            except Exception as e:
                logger.error(f"Error closing Xray process: {e}")

        # Cleanup Config File
        if self.config_path and os.path.exists(self.config_path):
            try:
                await asyncio.to_thread(os.remove, self.config_path)
            except Exception as e:
                logger.warning(f"Failed to remove temp config {self.config_path}: {e}")

    async def _wait_for_ports(self, ports: List[int], timeout: float) -> bool:
        """Waits until the first port is listening."""
        if not ports: return True
        start_time = time.time()
        port = ports[0]
        while time.time() - start_time < timeout:
            try:
                # check if process is still alive
                if self.process.returncode is not None:
                    return False
                
                reader, writer = await asyncio.open_connection('127.0.0.1', port)
                writer.close()
                await writer.wait_closed()
                return True
            except (ConnectionRefusedError, OSError):
                await asyncio.sleep(0.1)
        return False


class XrayService:
    def __init__(self, settings: Settings):
        self.settings = settings

    def build_xray_config_for_batch(self, servers: List[ProxyServer], base_port: int) -> Dict[str, Any]:
        inbounds, outbounds, routing_rules = [], [], []
        for i, server in enumerate(servers):
            inbound_port = base_port + i
            inbound_tag, outbound_tag = f"in-{i}", f"out-{i}"
            inbounds.append({
                "tag": inbound_tag, "port": inbound_port, "listen": "127.0.0.1", "protocol": "socks",
                "settings": {"auth": "noauth", "udp": True, "ip": "127.0.0.1"},
            })

            outbound_config = server.to_xray_outbound()
            if outbound_config:
                outbound_config["tag"] = outbound_tag
                outbounds.append(outbound_config)
                routing_rules.append({"type": "field", "inboundTag": [inbound_tag], "outboundTag": outbound_tag})

        return {"log": {"loglevel": "warning"}, "inbounds": inbounds, "outbounds": outbounds, "routing": {"rules": routing_rules}}

    async def _call_go_tester(self, xray_config: Dict[str, Any], test_url: str, ports: List[int], is_site_check: bool) -> List[Dict[str, Any]]:
        # Find the path to the Go binary
        go_binary_path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "go-tester", "xray-tester")
        
        request_payload = {
            "xray_config": xray_config,
            "xray_path": self.settings.XRAY_PATH,
            "xray_assets_path": self.settings.XRAY_ASSETS_PATH,
            "test_url": test_url,
            "timeout": self.settings.TEST_TIMEOUT,
            "ports": ports,
            "is_site_check": is_site_check
        }

        try:
            # Run the Go binary as a subprocess
            process = await asyncio.create_subprocess_exec(
                go_binary_path,
                stdin=asyncio.subprocess.PIPE,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE
            )
            
            stdout, stderr = await process.communicate(input=json.dumps(request_payload).encode('utf-8'))
            
            if process.returncode != 0:
                logger.error(f"Go tester failed with exit code {process.returncode}: {stderr.decode('utf-8')}")
                return []
                
            return json.loads(stdout.decode('utf-8'))
        except Exception as e:
            logger.error(f"Failed to execute Go tester: {e}")
            return []

    async def run_test_batch(self, servers: List[ProxyServer]) -> List[Tuple[ProxyServer, float]]:
        if not servers:
            return []

        xray_config = self.build_xray_config_for_batch(servers, self.settings.BASE_PORT)
        ports = [self.settings.BASE_PORT + i for i in range(len(servers))]

        results = await self._call_go_tester(
            xray_config, 
            self.settings.LATENCY_TEST_URL, 
            ports, 
            is_site_check=False
        )

        if not results:
            return [(s, float("inf")) for s in servers]

        # Map results back to servers
        delay_map = {res["port"]: res["delay"] if not res["failed"] else float("inf") for res in results}
        return [(s, delay_map.get(port, float("inf"))) for s, port in zip(servers, ports)]
    
    async def evaluate_site_accessibility(self, url: str, servers_to_test: List[ProxyServer]) -> List[ProxyServer]:
        """Helper to test a list of servers against a specific URL."""
        successful_servers = []
        
        for i in range(0, len(servers_to_test), self.settings.BATCH_SIZE):
            batch = servers_to_test[i : i + self.settings.BATCH_SIZE]
            logger.info(f"Testing batch {i // self.settings.BATCH_SIZE + 1} for site: {url}")

            xray_config = self.build_xray_config_for_batch(batch, self.settings.BASE_PORT)
            ports = [self.settings.BASE_PORT + j for j in range(len(batch))]

            results = await self._call_go_tester(
                xray_config, 
                url, 
                ports, 
                is_site_check=True
            )

            if results:
                success_ports = {res["port"] for res in results if not res["failed"]}
                for server, port in zip(batch, ports):
                    if port in success_ports:
                        successful_servers.append(server)
            else:
                logger.error("Site check batch failed or returned no results")
        
        return successful_servers

    def run_single_proxy(self, server: ProxyServer, port: int):
        """Returns a context manager that runs a single server as a proxy on the given port."""
        config = self.build_xray_config_for_batch([server], port)
        return XrayProcessContext(self.settings, config, [port], timeout=10.0)
