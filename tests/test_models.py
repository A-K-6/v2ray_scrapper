import unittest
import sys
import os

# Add src to path
sys.path.append(os.path.abspath(os.path.join(os.path.dirname(__file__), '../src')))

from models.server import (
    VlessServer, VmessServer, TrojanServer, 
    ShadowsocksServer, Hysteria2Server
)

class TestProtocolModels(unittest.TestCase):
    def test_vless_to_uri(self):
        s = VlessServer(
            remark="Test",
            address="example.com",
            port=443,
            vless_id="uuid",
            security="reality",
            sni="example.com",
            fp="chrome",
            pbk="pubkey",
            sid="shortid"
        )
        uri = s.to_uri()
        self.assertIn("vless://uuid@example.com:443", uri)
        self.assertIn("security=reality", uri)
        self.assertIn("sni=example.com", uri)
        self.assertIn("#Test", uri)

    def test_vless_to_xray_outbound(self):
        s = VlessServer(
            address="example.com",
            port=443,
            vless_id="uuid",
            security="reality",
            sni="example.com",
            fp="chrome",
            pbk="pubkey",
            sid="shortid"
        )
        outbound = s.to_xray_outbound()
        self.assertEqual(outbound["protocol"], "vless")
        self.assertEqual(outbound["streamSettings"]["security"], "reality")
        self.assertEqual(outbound["streamSettings"]["realitySettings"]["publicKey"], "pubkey")

    def test_vmess_to_uri(self):
        s = VmessServer(
            remark="VMessTest",
            address="example.com",
            port=443,
            vmess_id="uuid",
            type="ws",
            path="/path",
            tls="tls"
        )
        uri = s.to_uri()
        self.assertTrue(uri.startswith("vmess://"))
        # Base64 decode and check
        import base64, json
        decoded = json.loads(base64.b64decode(uri.replace("vmess://", "")).decode())
        self.assertEqual(decoded["ps"], "VMessTest")
        self.assertEqual(decoded["add"], "example.com")
        self.assertEqual(decoded["net"], "ws")

    def test_trojan_to_xray_outbound(self):
        s = TrojanServer(
            address="example.com",
            port=443,
            password="pass",
            sni="sni.com"
        )
        outbound = s.to_xray_outbound()
        self.assertEqual(outbound["protocol"], "trojan")
        self.assertEqual(outbound["settings"]["servers"][0]["password"], "pass")
        self.assertEqual(outbound["streamSettings"]["tlsSettings"]["serverName"], "sni.com")

    def test_ss_to_uri(self):
        s = ShadowsocksServer(
            remark="SSTest",
            address="example.com",
            port=8388,
            method="aes-256-gcm",
            password="pass"
        )
        uri = s.to_uri()
        self.assertTrue(uri.startswith("ss://"))
        self.assertIn("#SSTest", uri)

    def test_hy2_to_uri(self):
        s = Hysteria2Server(
            remark="Hy2Test",
            address="example.com",
            port=1234,
            password="pass",
            insecure=True
        )
        uri = s.to_uri()
        self.assertIn("hy2://pass@example.com:1234", uri)
        self.assertIn("insecure=1", uri)

    def test_fingerprint_consistency(self):
        s1 = VlessServer(address="a", port=1, vless_id="u")
        s2 = VlessServer(address="a", port=1, vless_id="u", remark="different")
        self.assertEqual(s1.fingerprint, s2.fingerprint)
        
        s3 = VlessServer(address="a", port=2, vless_id="u")
        self.assertNotEqual(s1.fingerprint, s3.fingerprint)

if __name__ == '__main__':
    unittest.main()
