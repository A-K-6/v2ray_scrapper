package com.example.v2rayupdater

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.content.SharedPreferences
import android.net.Uri
import android.os.Bundle
import android.util.Base64
import android.widget.Toast
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ArrowForward
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Info
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import kotlinx.coroutines.*
import org.json.JSONObject
import java.io.BufferedReader
import java.io.InputStreamReader
import java.net.HttpURLConnection
import java.net.InetSocketAddress
import java.net.Socket
import java.net.URL

// Model definition
data class ProxyNode(
    val rawUri: String,
    val protocol: String,
    val remark: String,
    val host: String,
    val port: Int,
    var localLatency: Long = -2 // -2: untested, -1: failed/timeout, >=0: latency in ms
)

class MainActivity : ComponentActivity() {
    private lateinit var sharedPreferences: SharedPreferences

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        sharedPreferences = getSharedPreferences("v2ray_updater_prefs", Context.MODE_PRIVATE)

        setContent {
            V2RayUpdaterTheme {
                MainScreen(
                    sharedPreferences = sharedPreferences,
                    onCopyToClipboard = { label, text -> copyToClipboard(label, text) },
                    onLaunchClient = { packageName -> launchClientApp(packageName) }
                )
            }
        }
    }

    private fun copyToClipboard(label: String, text: String) {
        val clipboard = getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
        val clip = ClipData.newPlainText(label, text)
        clipboard.setPrimaryClip(clip)
        Toast.makeText(this, "Copied to clipboard", Toast.LENGTH_SHORT).show()
    }

    private fun launchClientApp(packageName: String) {
        val launchIntent = packageManager.getLaunchIntentForPackage(packageName)
        if (launchIntent != null) {
            startActivity(launchIntent)
        } else {
            Toast.makeText(this, "Client app is not installed. Package: $packageName", Toast.LENGTH_LONG).show()
            try {
                // Try opening in Play Store
                startActivity(Intent(Intent.ACTION_VIEW, Uri.parse("market://details?id=$packageName")))
            } catch (e: Exception) {
                // If Play Store isn't installed, open browser
                startActivity(Intent(Intent.ACTION_VIEW, Uri.parse("https://play.google.com/store/apps/details?id=$packageName")))
            }
        }
    }
}

// Color Palette for Dark Premium design
val DeepBg = Color(0xFF0F0E17)
val CardBg = Color(0xFF1F1E29)
val PrimaryNeon = Color(0xFF9F7AEA)
val SecondaryNeon = Color(0xFF3182CE)
val TextPrimary = Color(0xFFFFFFFE)
val TextSecondary = Color(0xFFA0AEC0)
val SuccessGreen = Color(0xFF48BB78)
val DangerRed = Color(0xFFF56565)
val WarningYellow = Color(0xFFECC94B)

@Composable
fun V2RayUpdaterTheme(content: @Composable () -> Unit) {
    MaterialTheme(
        colorScheme = darkColorScheme(
            background = DeepBg,
            surface = CardBg,
            primary = PrimaryNeon,
            secondary = SecondaryNeon,
            onBackground = TextPrimary,
            onSurface = TextPrimary
        ),
        content = content
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun MainScreen(
    sharedPreferences: SharedPreferences,
    onCopyToClipboard: (String, String) -> Unit,
    onLaunchClient: (String) -> Unit
) {
    val coroutineScope = rememberCoroutineScope()
    var subscriptionUrl by remember {
        mutableStateOf(sharedPreferences.getString("sub_url", "https://raw.githubusercontent.com/freefq/free/master/v2") ?: "")
    }
    var rawConfigData by remember { mutableStateOf("") }
    var nodesList by remember { mutableStateOf(listOf<ProxyNode>()) }
    var isLoading by remember { mutableStateOf(false) }
    var statusMessage by remember { mutableStateOf("Enter URL and Fetch Subscription") }
    var isTestingLatencies by remember { mutableStateOf(false) }
    var searchQuery by remember { mutableStateOf("") }

    // Save URL whenever it changes
    LaunchedEffect(subscriptionUrl) {
        sharedPreferences.edit().putString("sub_url", subscriptionUrl).apply()
    }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .background(
                brush = Brush.verticalGradient(
                    colors = listOf(DeepBg, Color(0xFF1A1625))
                )
            )
            .padding(16.dp)
    ) {
        // App Title
        Text(
            text = "⚡ V2Ray Updater",
            fontSize = 24.sp,
            fontWeight = FontWeight.Bold,
            color = TextPrimary,
            modifier = Modifier.padding(bottom = 16.dp)
        )

        // Configuration Card
        Card(
            modifier = Modifier
                .fillMaxWidth()
                .padding(bottom = 16.dp)
                .border(1.dp, Color(0xFF2D2B3D), RoundedCornerShape(12.dp)),
            colors = CardDefaults.cardColors(containerColor = CardBg)
        ) {
            Column(modifier = Modifier.padding(14.dp)) {
                Text(
                    text = "Subscription / API Endpoint",
                    fontWeight = FontWeight.SemiBold,
                    fontSize = 14.sp,
                    color = PrimaryNeon,
                    modifier = Modifier.padding(bottom = 8.dp)
                )

                OutlinedTextField(
                    value = subscriptionUrl,
                    onValueChange = { subscriptionUrl = it },
                    placeholder = { Text("https://example.com/sub", color = Color.Gray) },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                    colors = TextFieldDefaults.outlinedTextFieldColors(
                        textColor = TextPrimary,
                        focusedBorderColor = PrimaryNeon,
                        unfocusedBorderColor = Color(0xFF3F3B5C),
                        containerColor = Color(0xFF14131D)
                    )
                )

                Spacer(modifier = Modifier.height(10.dp))

                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    Button(
                        onClick = {
                            isLoading = true
                            statusMessage = "Fetching..."
                            coroutineScope.launch {
                                val fetched = fetchSubscription(subscriptionUrl)
                                isLoading = false
                                if (fetched != null) {
                                    rawConfigData = fetched
                                    val parsed = parseSubscription(fetched)
                                    nodesList = parsed
                                    statusMessage = "Fetched ${parsed.size} working nodes!"
                                } else {
                                    statusMessage = "Failed to fetch configurations."
                                }
                            }
                        },
                        enabled = !isLoading && subscriptionUrl.isNotBlank(),
                        modifier = Modifier.weight(1f),
                        colors = ButtonDefaults.buttonColors(containerColor = PrimaryNeon)
                    ) {
                        if (isLoading) {
                            CircularProgressIndicator(size = 20.dp, color = Color.White)
                        } else {
                            Icon(Icons.Default.Refresh, contentDescription = "Fetch")
                            Spacer(modifier = Modifier.width(6.dp))
                            Text("Fetch Nodes", fontWeight = FontWeight.Bold)
                        }
                    }

                    Button(
                        onClick = {
                            onCopyToClipboard("V2Ray Sub Link", subscriptionUrl)
                        },
                        modifier = Modifier.weight(1f),
                        colors = ButtonDefaults.buttonColors(containerColor = SecondaryNeon)
                    ) {
                        Text("Copy Link")
                    }
                }
            }
        }

        // Status Card & Quick Actions
        if (nodesList.isNotEmpty()) {
            Card(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(bottom = 12.dp)
                    .border(1.dp, Color(0xFF2D2B3D), RoundedCornerShape(12.dp)),
                colors = CardDefaults.cardColors(containerColor = CardBg)
            ) {
                Column(modifier = Modifier.padding(12.dp)) {
                    Text(
                        text = statusMessage,
                        fontSize = 14.sp,
                        color = SuccessGreen,
                        fontWeight = FontWeight.Medium
                    )

                    Spacer(modifier = Modifier.height(8.dp))

                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        Button(
                            onClick = {
                                if (!isTestingLatencies) {
                                    isTestingLatencies = true
                                    coroutineScope.launch {
                                        testAllNodes(nodesList) { updatedNode ->
                                            // Trigger UI recomposition
                                            nodesList = nodesList.toList()
                                        }
                                        isTestingLatencies = false
                                        statusMessage = "Local TCP latency test completed!"
                                    }
                                }
                            },
                            enabled = !isTestingLatencies,
                            colors = ButtonDefaults.buttonColors(containerColor = Color(0xFF4A5568)),
                            modifier = Modifier.weight(1.2f)
                        ) {
                            if (isTestingLatencies) {
                                CircularProgressIndicator(size = 18.dp, color = Color.White)
                            } else {
                                Icon(Icons.Default.PlayArrow, contentDescription = "Test")
                                Spacer(modifier = Modifier.width(4.dp))
                                Text("TCP Ping", fontSize = 12.sp)
                            }
                        }

                        Button(
                            onClick = {
                                val allConfigs = nodesList.joinToString("\n") { it.rawUri }
                                onCopyToClipboard("V2Ray Configs", allConfigs)
                            },
                            colors = ButtonDefaults.buttonColors(containerColor = Color(0xFF2D3748)),
                            modifier = Modifier.weight(1f)
                        ) {
                            Text("Copy All", fontSize = 12.sp)
                        }
                    }
                }
            }
        }

        // Launch Clients Section
        Text(
            text = "Open Client App",
            fontSize = 14.sp,
            fontWeight = FontWeight.SemiBold,
            color = TextSecondary,
            modifier = Modifier.padding(bottom = 8.dp)
        )

        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(bottom = 12.dp),
            horizontalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            Button(
                onClick = { onLaunchClient("ang.go.v2art") },
                colors = ButtonDefaults.buttonColors(containerColor = CardBg),
                modifier = Modifier
                    .weight(1f)
                    .border(1.dp, Color(0xFF3F3B5C), RoundedCornerShape(8.dp)),
                contentPadding = PaddingValues(8.dp)
            ) {
                Text("v2rayNG", fontSize = 12.sp, color = TextPrimary)
            }
            Button(
                onClick = { onLaunchClient("moe.nb.nekobox") },
                colors = ButtonDefaults.buttonColors(containerColor = CardBg),
                modifier = Modifier
                    .weight(1f)
                    .border(1.dp, Color(0xFF3F3B5C), RoundedCornerShape(8.dp)),
                contentPadding = PaddingValues(8.dp)
            ) {
                Text("Nekobox", fontSize = 12.sp, color = TextPrimary)
            }
            Button(
                onClick = { onLaunchClient("io.nekohasekai.singbox") },
                colors = ButtonDefaults.buttonColors(containerColor = CardBg),
                modifier = Modifier
                    .weight(1f)
                    .border(1.dp, Color(0xFF3F3B5C), RoundedCornerShape(8.dp)),
                contentPadding = PaddingValues(8.dp)
            ) {
                Text("Sing-Box", fontSize = 12.sp, color = TextPrimary)
            }
        }

        // Search & Filter
        if (nodesList.isNotEmpty()) {
            OutlinedTextField(
                value = searchQuery,
                onValueChange = { searchQuery = it },
                placeholder = { Text("Filter by name or protocol...", color = Color.Gray, fontSize = 13.sp) },
                singleLine = true,
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(bottom = 8.dp),
                colors = TextFieldDefaults.outlinedTextFieldColors(
                    textColor = TextPrimary,
                    focusedBorderColor = SecondaryNeon,
                    unfocusedBorderColor = Color(0xFF2D2B3D),
                    containerColor = Color(0xFF14131D)
                )
            )
        }

        // Nodes List
        val filteredNodes = nodesList.filter {
            it.remark.contains(searchQuery, ignoreCase = true) ||
                    it.protocol.contains(searchQuery, ignoreCase = true)
        }

        LazyColumn(
            verticalArrangement = Arrangement.spacedBy(8.dp),
            modifier = Modifier.weight(1f)
        ) {
            items(filteredNodes) { node ->
                NodeCard(node = node, onCopy = { onCopyToClipboard("V2Ray Node", node.rawUri) })
            }
        }
    }
}

@Composable
fun NodeCard(node: ProxyNode, onCopy: () -> Unit) {
    Card(
        modifier = Modifier
            .fillMaxWidth()
            .border(1.dp, Color(0xFF2D2B3D), RoundedCornerShape(10.dp)),
        colors = CardDefaults.cardColors(containerColor = Color(0xFF161520))
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(12.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            // Protocol Tag
            Box(
                modifier = Modifier
                    .clip(RoundedCornerShape(4.dp))
                    .background(
                        when (node.protocol) {
                            "vless" -> Color(0xFF805AD5)
                            "vmess" -> Color(0xFF3182CE)
                            "trojan" -> Color(0xFFDD6B20)
                            "ss" -> Color(0xFF319795)
                            else -> Color(0xFF718096)
                        }
                    )
                    .padding(horizontal = 6.dp, vertical = 2.dp)
            ) {
                Text(
                    text = node.protocol.uppercase(),
                    fontSize = 10.sp,
                    fontWeight = FontWeight.Bold,
                    color = Color.White
                )
            }

            Spacer(modifier = Modifier.width(10.dp))

            // Remark and Host details
            Column(modifier = Modifier.weight(1f)) {
                Text(
                    text = node.remark,
                    fontSize = 14.sp,
                    fontWeight = FontWeight.SemiBold,
                    color = TextPrimary,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis
                )
                Text(
                    text = "${node.host}:${node.port}",
                    fontSize = 11.sp,
                    color = TextSecondary,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    fontFamily = FontFamily.Monospace
                )
            }

            Spacer(modifier = Modifier.width(8.dp))

            // Local Latency Badge
            Box(
                modifier = Modifier
                    .clip(RoundedCornerShape(4.dp))
                    .background(
                        when {
                            node.localLatency == -2L -> Color(0xFF2D3748)
                            node.localLatency == -1L -> Color(0xFF5A2020)
                            node.localLatency < 150L -> Color(0xFF1C452D)
                            node.localLatency < 300L -> Color(0xFF744210)
                            else -> Color(0xFF5A2020)
                        }
                    )
                    .padding(horizontal = 6.dp, vertical = 4.dp)
            ) {
                Text(
                    text = when {
                        node.localLatency == -2L -> "Untested"
                        node.localLatency == -1L -> "Fail"
                        else -> "${node.localLatency}ms"
                    },
                    fontSize = 10.sp,
                    fontWeight = FontWeight.Bold,
                    color = when {
                        node.localLatency == -2L -> TextSecondary
                        node.localLatency == -1L -> DangerRed
                        node.localLatency < 150L -> SuccessGreen
                        node.localLatency < 300L -> WarningYellow
                        else -> DangerRed
                    }
                )
            }

            Spacer(modifier = Modifier.width(10.dp))

            // Copy Action
            Button(
                onClick = onCopy,
                colors = ButtonDefaults.buttonColors(containerColor = Color(0xFF2D3748)),
                contentPadding = PaddingValues(horizontal = 10.dp, vertical = 2.dp),
                modifier = Modifier.height(28.dp)
            ) {
                Text("Copy", fontSize = 10.sp)
            }
        }
    }
}

// Network Helpers
suspend fun fetchSubscription(urlStr: String): String? = withContext(Dispatchers.IO) {
    try {
        val url = URL(urlStr)
        val conn = url.openConnection() as HttpURLConnection
        conn.requestMethod = "GET"
        conn.connectTimeout = 8000
        conn.readTimeout = 8000
        conn.setRequestProperty("User-Agent", "v2rayNG/1.8.5")

        if (conn.responseCode == 200) {
            val reader = BufferedReader(InputStreamReader(conn.inputStream))
            val sb = java.lang.StringBuilder()
            var line: String?
            while (reader.readLine().also { line = it } != null) {
                sb.append(line).append("\n")
            }
            reader.close()
            sb.toString()
        } else {
            null
        }
    } catch (e: Exception) {
        e.printStackTrace()
        null
    }
}

fun parseSubscription(rawData: String): List<ProxyNode> {
    val nodes = mutableListOf<ProxyNode>()
    try {
        // Subscriptions are typically Base64 encoded lists of URIs (newline separated)
        // Let's check if the raw data is Base64 encoded
        val cleanData = rawData.trim()
        val decodedData = try {
            val decodedBytes = Base64.decode(cleanData, Base64.DEFAULT)
            String(decodedBytes)
        } catch (e: Exception) {
            // If it fails to decode, try decoding without padding or whitespace
            try {
                val decodedBytes = Base64.decode(cleanData.replace("\n", "").replace("\r", ""), Base64.DEFAULT)
                String(decodedBytes)
            } catch (ex: Exception) {
                // If it's not base64, assume it is raw plaintext configs (newline separated)
                rawData
            }
        }

        val lines = decodedData.split("\n")
        for (line in lines) {
            val trimmed = line.trim()
            if (trimmed.isNotBlank()) {
                val parsed = parseProxyUri(trimmed)
                if (parsed != null) {
                    nodes.add(parsed)
                }
            }
        }
    } catch (e: Exception) {
        e.printStackTrace()
    }
    return nodes
}

fun parseProxyUri(uri: String): ProxyNode? {
    try {
        val protocol = uri.substringBefore("://").lowercase()
        if (protocol == "vmess") {
            val base64Part = uri.substringAfter("vmess://")
            val decoded = String(Base64.decode(base64Part, Base64.DEFAULT))
            val json = JSONObject(decoded)
            return ProxyNode(
                rawUri = uri,
                protocol = protocol,
                remark = json.optString("ps", "VMess Node"),
                host = json.optString("add"),
                port = json.optInt("port", 443)
            )
        } else {
            val parts = uri.split("#")
            val addressPart = parts[0]
            val remark = if (parts.size > 1) Uri.decode(parts[1]) else "Node"

            val cleanAddress = addressPart.substringAfter("://")
            // Extract host and port
            val hostPortPart = if (cleanAddress.contains("@")) {
                cleanAddress.substringAfter("@").substringBefore("?")
            } else {
                cleanAddress.substringBefore("?")
            }

            val host = hostPortPart.substringBeforeLast(":")
            val portStr = hostPortPart.substringAfterLast(":")
            val port = portStr.toIntOrNull() ?: 443

            return ProxyNode(
                rawUri = uri,
                protocol = protocol,
                remark = remark,
                host = host,
                port = port
            )
        }
    } catch (e: Exception) {
        return null
    }
}

// Local latency testing
suspend fun testTcpLatency(host: String, port: Int, timeoutMs: Int = 3000): Long {
    return withContext(Dispatchers.IO) {
        val startTime = System.currentTimeMillis()
        try {
            Socket().use { socket ->
                socket.connect(InetSocketAddress(host, port), timeoutMs)
            }
            System.currentTimeMillis() - startTime
        } catch (e: Exception) {
            -1L // Connection failed or timeout
        }
    }
}

suspend fun testAllNodes(nodes: List<ProxyNode>, onProgress: (ProxyNode) -> Unit) = coroutineScope {
    // Process in parallel batches of 15 to avoid overloading mobile networks
    nodes.chunked(15).forEach { batch ->
        val jobs = batch.map { node ->
            async {
                val latency = testTcpLatency(node.host, node.port)
                node.localLatency = latency
                onProgress(node)
            }
        }
        jobs.awaitAll()
    }
}
