package main

const swaggerHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>V2Ray Scrapper API</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    SwaggerUIBundle({url: "/openapi.json", dom_id: "#swagger-ui", deepLinking: true, displayRequestDuration: true});
  </script>
</body>
</html>`

const openAPISpec = `{
  "openapi": "3.1.0",
  "info": {
    "title": "V2Ray Scrapper API",
    "description": "Scrape, test, cache, and distribute working proxy configurations.",
    "version": "2.1.0"
  },
  "servers": [{"url": "/"}],
  "tags": [
    {"name": "System"},
    {"name": "Cache"},
    {"name": "Testing"},
    {"name": "Management"}
  ],
  "paths": {
    "/health": {
      "get": {"tags":["System"],"summary":"Check service health","operationId":"health","responses":{"200":{"description":"Service is healthy","content":{"application/json":{"schema":{"$ref":"#/components/schemas/HealthResponse"}}}}}}
    },
    "/servers/live": {
      "get": {"tags":["Testing"],"summary":"Start a background refresh and return the current top 25","operationId":"refreshServers","responses":{"200":{"description":"Current cache and refresh status","content":{"application/json":{"schema":{"$ref":"#/components/schemas/ServerResponse"}}}}}}
    },
    "/cache": {
      "get": {"tags":["Cache"],"summary":"Get the top 25 working servers","operationId":"getCache","responses":{"200":{"description":"Cached servers","content":{"application/json":{"schema":{"$ref":"#/components/schemas/ServerResponse"}}}},"503":{"$ref":"#/components/responses/CacheUnavailable"}}}
    },
    "/cache/raw": {
      "get": {"tags":["Cache"],"summary":"Get the top 25 as newline-separated URIs","operationId":"getRawCache","responses":{"200":{"description":"Raw subscription","content":{"text/plain":{"schema":{"type":"string"}}}},"503":{"$ref":"#/components/responses/CacheUnavailable"}}}
    },
    "/cache/base64": {
      "get": {"tags":["Cache"],"summary":"Get the top 25 as a Base64 subscription","operationId":"getBase64Cache","parameters":[{"$ref":"#/components/parameters/Country"}],"responses":{"200":{"description":"Base64 subscription","content":{"text/plain":{"schema":{"type":"string","contentEncoding":"base64"}}}},"503":{"$ref":"#/components/responses/CacheUnavailable"}}}
    },
    "/cache/all/base64": {
      "get": {"tags":["Cache"],"summary":"Get every working server as a Base64 subscription","operationId":"getAllBase64Cache","parameters":[{"$ref":"#/components/parameters/Country"}],"responses":{"200":{"description":"Base64 subscription","content":{"text/plain":{"schema":{"type":"string","contentEncoding":"base64"}}}},"503":{"$ref":"#/components/responses/CacheUnavailable"}}}
    },
    "/subscription/site-specific": {
      "get": {"tags":["Testing"],"summary":"Test cached servers against a target site","operationId":"testSite","parameters":[{"name":"url","in":"query","required":true,"schema":{"type":"string","format":"uri"}},{"$ref":"#/components/parameters/Country"}],"responses":{"200":{"description":"Base64 subscription","content":{"text/plain":{"schema":{"type":"string","contentEncoding":"base64"}}}},"400":{"$ref":"#/components/responses/BadRequest"},"404":{"description":"No working server for the target","content":{"application/json":{"schema":{"$ref":"#/components/schemas/ErrorResponse"}}}},"429":{"$ref":"#/components/responses/Busy"},"503":{"$ref":"#/components/responses/CacheUnavailable"}}}
    },
    "/subscription/test": {
      "post": {"tags":["Testing"],"summary":"Test raw or Base64 subscription content","operationId":"testSubscription","requestBody":{"required":true,"content":{"text/plain":{"schema":{"type":"string"}}}},"responses":{"200":{"description":"Working servers","content":{"application/json":{"schema":{"$ref":"#/components/schemas/ServerResponse"}}}},"429":{"$ref":"#/components/responses/Busy"}}}
    },
    "/subscription/test-custom": {
      "post": {"tags":["Testing"],"summary":"Test custom sources and content","operationId":"testCustomSubscription","requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/CustomTestRequest"}}}},"responses":{"200":{"description":"Working servers","content":{"application/json":{"schema":{"$ref":"#/components/schemas/ServerResponse"}}}},"400":{"$ref":"#/components/responses/BadRequest"},"429":{"$ref":"#/components/responses/Busy"}}}
    },
    "/subscriptions": {
      "get": {"tags":["Management"],"summary":"List managed subscription sources","operationId":"listSubscriptions","security":[{"ManagementApiKey":[]},{"ManagementBearer":[]}],"responses":{"200":{"description":"Subscription sources"},"401":{"description":"Unauthorized"},"503":{"description":"Management unavailable"}}},
      "post": {"tags":["Management"],"summary":"Add managed subscription sources","operationId":"addSubscriptions","security":[{"ManagementApiKey":[]},{"ManagementBearer":[]}],"requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/SubscriptionsRequest"}}}},"responses":{"200":{"description":"Updated sources"},"400":{"$ref":"#/components/responses/BadRequest"},"401":{"description":"Unauthorized"},"503":{"description":"Management unavailable"}}},
      "delete": {"tags":["Management"],"summary":"Remove managed subscription sources","operationId":"removeSubscriptions","security":[{"ManagementApiKey":[]},{"ManagementBearer":[]}],"requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/SubscriptionsRequest"}}}},"responses":{"200":{"description":"Updated sources"},"400":{"$ref":"#/components/responses/BadRequest"},"401":{"description":"Unauthorized"},"503":{"description":"Management unavailable"}}}
    },
    "/sites": {
      "get": {"tags":["Management"],"summary":"List managed preloaded site checks","operationId":"listSites","security":[{"ManagementApiKey":[]},{"ManagementBearer":[]}],"responses":{"200":{"description":"Configured sites"},"401":{"description":"Unauthorized"},"503":{"description":"Management unavailable"}}},
      "post": {"tags":["Management"],"summary":"Add or update a preloaded site check","operationId":"putSite","security":[{"ManagementApiKey":[]},{"ManagementBearer":[]}],"requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/SiteRequest"}}}},"responses":{"201":{"description":"Site stored"},"400":{"$ref":"#/components/responses/BadRequest"},"401":{"description":"Unauthorized"},"503":{"description":"Management unavailable"}}},
      "delete": {"tags":["Management"],"summary":"Remove a preloaded site check","operationId":"removeSite","security":[{"ManagementApiKey":[]},{"ManagementBearer":[]}],"requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/SiteRequest"}}}},"responses":{"204":{"description":"Site removed"},"400":{"$ref":"#/components/responses/BadRequest"},"401":{"description":"Unauthorized"},"503":{"description":"Management unavailable"}}}
    }
  },
  "components": {
    "securitySchemes": {
      "ManagementApiKey": {"type":"apiKey","in":"header","name":"X-API-Key"},
      "ManagementBearer": {"type":"http","scheme":"bearer"}
    },
    "parameters": {
      "Country": {"name":"country","in":"query","required":false,"description":"Comma-separated ISO country codes, such as US,DE","schema":{"type":"string"}}
    },
    "responses": {
      "BadRequest": {"description":"Invalid request","content":{"application/json":{"schema":{"$ref":"#/components/schemas/ErrorResponse"}}}},
      "Busy": {"description":"Another test is running","content":{"application/json":{"schema":{"$ref":"#/components/schemas/ErrorResponse"}}}},
      "CacheUnavailable": {"description":"Cache has not been initialized","content":{"application/json":{"schema":{"$ref":"#/components/schemas/ErrorResponse"}}}}
    },
    "schemas": {
      "HealthResponse": {"type":"object","required":["status"],"properties":{"status":{"type":"string","examples":["ok"]}}},
      "ErrorResponse": {"type":"object","required":["detail"],"properties":{"detail":{"type":"string"}}},
      "ServerResponse": {"type":"object","required":["count","servers"],"properties":{"count":{"type":"integer","minimum":0},"servers":{"type":"array","items":{"$ref":"#/components/schemas/ProxyServer"}},"message":{"type":"string"}}},
      "CustomTestRequest": {"type":"object","properties":{"subscription_urls":{"type":"array","items":{"type":"string","format":"uri"}},"custom_content":{"type":"string"},"test_url":{"type":"string","format":"uri"},"max_delay_ms":{"type":"integer","minimum":1},"limit":{"type":"integer","minimum":1,"maximum":500,"default":50}}},
      "SubscriptionsRequest": {"type":"object","required":["urls"],"properties":{"urls":{"type":"array","minItems":1,"items":{"type":"string","format":"uri"}}}},
      "SiteRequest": {"type":"object","required":["url"],"properties":{"url":{"type":"string","format":"uri"},"filename":{"type":"string"}}},
      "ProxyServer": {"type":"object","required":["protocol","address","port","delay","country_code","flag","raw_uri"],"properties":{"protocol":{"type":"string","enum":["vless","vmess","trojan","shadowsocks","hysteria2"]},"remark":{"type":"string"},"address":{"type":"string"},"port":{"type":"integer","minimum":1,"maximum":65535},"delay":{"type":"integer"},"country_code":{"type":"string"},"flag":{"type":"string"},"fail_count":{"type":"integer","minimum":0},"raw_uri":{"type":"string"},"vless_id":{"type":"string"},"vmess_id":{"type":"string"},"encryption":{"type":"string"},"security":{"type":"string"},"type":{"type":"string"},"host":{"type":"string"},"path":{"type":"string"},"sni":{"type":"string"},"flow":{"type":"string"},"fp":{"type":"string"},"pbk":{"type":"string"},"sid":{"type":"string"},"aid":{"type":"integer"},"method":{"type":"string"},"password":{"type":"string"},"insecure":{"type":"boolean"},"obfs":{"type":"string"},"obfs_password":{"type":"string"}}}
    }
  }
}`
