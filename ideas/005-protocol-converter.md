# Idea: Protocol Converter (Clash/Sing-box)

## Context
The system outputs standard V2Ray links (vless://, vmess://). Many users use Clash, Meta, or Sing-box clients which require specific YAML or JSON configuration formats.

## Proposal
Add endpoints to export the working server list in Clash and Sing-box formats.

## Implementation Steps
1.  **Clash Support:**
    -   Create a utility `src/utils/clash_converter.py`.
    -   Map VLESS/VMess fields to Clash Proxy Group syntax.
    -   Endpoint: `GET /subscribe/clash` -> returns YAML.
2.  **Sing-box Support:**
    -   Map fields to Sing-box outbound JSON structure.
    -   Endpoint: `GET /subscribe/sing-box` -> returns JSON.

## Benefits
-   **Compatibility:** Supports a wider range of clients (iOS, Mac, Android specific apps).
-   **Convenience:** Users don't need external converters.
