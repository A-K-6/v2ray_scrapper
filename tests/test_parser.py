import sys
import os
import unittest

# Add src to path
sys.path.append(os.path.abspath(os.path.join(os.path.dirname(__file__), '../src')))

from service.parse_uri import ProxyParser

class TestProxyParser(unittest.TestCase):
    def setUp(self):
        self.parser = ProxyParser()

    def test_parse_vless(self):
        uri = "vless://uuid@example.com:443?security=reality&sni=example.com&fp=chrome&pbk=publickey&sid=shortid&type=tcp&flow=xtls-rprx-vision#Example"
        result = self.parser.parse(uri)
        self.assertIsNotNone(result)
        self.assertEqual(result['protocol'], 'vless')
        self.assertEqual(result['address'], 'example.com')
        self.assertEqual(result['port'], 443)
        self.assertEqual(result['vless_id'], 'uuid')
        self.assertEqual(result['security'], 'reality')
        self.assertEqual(result['remark'], 'Example')

    def test_parse_trojan(self):
        uri = "trojan://password@example.com:443?security=tls&sni=example.com&type=tcp#Trojan"
        result = self.parser.parse(uri)
        self.assertIsNotNone(result)
        self.assertEqual(result['protocol'], 'trojan')
        self.assertEqual(result['address'], 'example.com')
        self.assertEqual(result['port'], 443)
        self.assertEqual(result['password'], 'password')
        self.assertEqual(result['remark'], 'Trojan')

    def test_parse_ss(self):
        # ss://Y2hhY2hhMjAtaWV0Zi1wb2x5MTMwNTpwYXNzd29yZA==@example.com:8388#Shadowsocks
        # base64 decode of chacha20-ietf-poly1305:password is Y2hhY2hhMjAtaWV0Zi1wb2x5MTMwNTpwYXNzd29yZA==
        uri = "ss://Y2hhY2hhMjAtaWV0Zi1wb2x5MTMwNTpwYXNzd29yZA==@example.com:8388#Shadowsocks"
        result = self.parser.parse(uri)
        self.assertIsNotNone(result)
        self.assertEqual(result['protocol'], 'shadowsocks')
        self.assertEqual(result['address'], 'example.com')
        self.assertEqual(result['port'], 8388)
        self.assertEqual(result['method'], 'chacha20-ietf-poly1305')
        self.assertEqual(result['password'], 'password')
        self.assertEqual(result['remark'], 'Shadowsocks')

    def test_parse_vmess(self):
        # vmess requires a JSON blob in base64. 
        import base64
        import json
        vmess_data = {
            "v": "2",
            "ps": "VMess",
            "add": "example.com",
            "port": "443",
            "id": "uuid",
            "aid": "0",
            "scy": "auto",
            "net": "ws",
            "type": "none",
            "host": "example.com",
            "path": "/path",
            "tls": "tls",
            "sni": "example.com",
            "alpn": ""
        }
        vmess_b64 = base64.b64encode(json.dumps(vmess_data).encode()).decode()
        uri = f"vmess://{vmess_b64}"
        
        result = self.parser.parse(uri)
        self.assertIsNotNone(result)
        self.assertEqual(result['protocol'], 'vmess')
        self.assertEqual(result['remark'], 'VMess')
        self.assertEqual(result['address'], 'example.com')
        self.assertEqual(result['port'], 443)
        self.assertEqual(result['vmess_id'], 'uuid')

    def test_parse_hy2(self):
        uri = "hy2://freehomesvpnchannel3@channel2.saghetalaie.homes:46914/?insecure=1&sni=www.google.com&obfs=salamander&obfs-password=%26O%2328YB5qK%215t%23U#TestHy2"
        result = self.parser.parse(uri)
        self.assertIsNotNone(result)
        self.assertEqual(result['protocol'], 'hysteria2')
        self.assertEqual(result['address'], 'channel2.saghetalaie.homes')
        self.assertEqual(result['port'], 46914)
        self.assertEqual(result['password'], 'freehomesvpnchannel3')
        self.assertEqual(result['sni'], 'www.google.com')
        self.assertTrue(result['insecure'])
        self.assertEqual(result['obfs'], 'salamander')
        self.assertEqual(result['obfs_password'], '&O#28YB5qK!5t#U')
        self.assertEqual(result['remark'], 'TestHy2')

if __name__ == '__main__':
    unittest.main()
