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

        return {"log": {"loglevel": "info"}, "inbounds": inbounds, "outbounds": outbounds, "routing": {"rules": routing_rules}}

    async def _call_go_tester(self, raw_uris: List[str] = None, sub_urls: List[str] = None, command: str = "test", test_url: str = None, base_port: int = None, is_site_check: bool = False) -> List[Dict[str, Any]]:
        # Find the path to the Go binary
        go_binary_path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "go-tester", "xray-tester")
        
        request_payload = {
            "command": command,
            "sub_urls": sub_urls or [],
            "raw_uris": raw_uris or [],
            "xray_path": self.settings.XRAY_PATH,
            "xray_assets_path": self.settings.XRAY_ASSETS_PATH,
            "test_url": test_url or self.settings.LATENCY_TEST_URL,
            "timeout": self.settings.TEST_TIMEOUT,
            "base_port": base_port or self.settings.BASE_PORT,
            "batch_size": self.settings.BATCH_SIZE,
            "max_parallel_batches": self.settings.MAX_CONCURRENT_BATCHES,
            "is_site_check": is_site_check,
            "state_path": self.settings.STATE_FILE_PATH
        }

        try:
            # Run the Go binary as a subprocess with a large buffer for massive JSON results
            process = await asyncio.create_subprocess_exec(
                go_binary_path,
                stdin=asyncio.subprocess.PIPE,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE,
                limit=10 * 1024 * 1024 # 10MB buffer
            )
            
            # Send payload and close stdin immediately
            payload_bytes = json.dumps(request_payload).encode('utf-8')
            process.stdin.write(payload_bytes)
            await process.stdin.drain()
            process.stdin.close()
            await process.stdin.wait_closed()

            # Create tasks to read stdout and stderr concurrently
            stdout_data = []

            async def stream_stderr():
                while True:
                    line = await process.stderr.readline()
                    if not line:
                        break
                    logger.info(f"[Go Tester] {line.decode('utf-8').strip()}")

            async def read_stdout():
                while True:
                    line = await process.stdout.readline()
                    if not line:
                        break
                    stdout_data.append(line.decode('utf-8'))

            # Run both streaming and waiting
            await asyncio.gather(stream_stderr(), read_stdout(), process.wait())
            
            full_stdout = "".join(stdout_data).strip()

            if process.returncode != 0:
                logger.error(f"Go tester failed with exit code {process.returncode}")
                return []
                
            if not full_stdout:
                return []
                
            try:
                return json.loads(full_stdout)
            except json.JSONDecodeError as e:
                logger.error(f"Failed to parse Go tester output as JSON: {e}. Raw: {full_stdout[:100]}...")
                return []
                
        except Exception as e:
            logger.error(f"Failed to execute Go tester: {e}")
            return []

    async def scrape_and_test(self, sub_urls: List[str], base_port: Optional[int] = None) -> List[Dict[str, Any]]:
        """Scrapes and tests servers entirely in Go."""
        return await self._call_go_tester(
            command="scrape-and-test",
            sub_urls=sub_urls,
            base_port=base_port
        )

    async def run_test_batch(self, servers: List[ProxyServer], base_port: Optional[int] = None, test_url: Optional[str] = None) -> List[Tuple[ProxyServer, float]]:
        if not servers:
            return []

        base_port = base_port or self.settings.BASE_PORT
        raw_uris = [s.raw_uri for s in servers]
        
        target_url = test_url or self.settings.LATENCY_TEST_URL
        is_site_check = False
        if test_url and test_url != self.settings.LATENCY_TEST_URL:
            is_site_check = True

        results = await self._call_go_tester(
            raw_uris=raw_uris, 
            test_url=target_url, 
            base_port=base_port, 
            is_site_check=is_site_check
        )

        if not results:
            return [(s, float("inf")) for s in servers]

        # Map results back to servers
        # results are returned in order of ports/uris
        delay_map = {res["port"]: res["delay"] if not res["failed"] else float("inf") for res in results}
        return [(s, delay_map.get(base_port + i, float("inf"))) for i, s in enumerate(servers)]
    
    async def evaluate_site_accessibility(self, url: str, servers_to_test: List[ProxyServer], base_port: Optional[int] = None) -> List[ProxyServer]:
        """Helper to test a list of servers against a specific URL."""
        successful_servers = []
        base_port = base_port or self.settings.BASE_PORT
        
        for i in range(0, len(servers_to_test), self.settings.BATCH_SIZE):
            batch = servers_to_test[i : i + self.settings.BATCH_SIZE]
            logger.info(f"Testing batch {i // self.settings.BATCH_SIZE + 1} for site: {url}")

            raw_uris = [s.raw_uri for s in batch]

            results = await self._call_go_tester(
                raw_uris=raw_uris, 
                test_url=url, 
                base_port=base_port, 
                is_site_check=True
            )

            if results:
                success_ports = {res["port"] for res in results if not res["failed"]}
                for j, server in enumerate(batch):
                    if (base_port + j) in success_ports:
                        successful_servers.append(server)
            else:
                logger.error("Site check batch failed or returned no results")
        
        return successful_servers

    def run_single_proxy(self, server: ProxyServer, port: int):
        """Returns a context manager that runs a single server as a proxy on the given port."""
        config = self.build_xray_config_for_batch([server], port)
        return XrayProcessContext(self.settings, config, [port], timeout=10.0)
