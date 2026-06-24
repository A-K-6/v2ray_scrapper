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
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.*
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
import org.json.JSONArray
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
                startActivity(Intent(Intent.ACTION_VIEW, Uri.parse("market://details?id=$packageName")))
            } catch (e: Exception) {
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
    var activeTab by remember { mutableStateOf(0) }

    // --- State Persistence & Helpers ---
    var coreApiUrl by remember {
        mutableStateOf(sharedPreferences.getString("core_api_url", "http://10.0.2.2:8084") ?: "http://10.0.2.2:8084")
    }
    
    // Subscriptions State
    val defaultSubs = """[
        "https://raw.githubusercontent.com/A-K-6/v2ray_scrapper_repo/tci_ir/subscription.txt",
        "https://raw.githubusercontent.com/Epodonios/v2ray-configs/main/All_Configs_Sub.txt",
        "https://raw.githubusercontent.com/barry-far/V2ray-config/main/All_Configs_base64_Sub.txt"
    ]""".trimIndent()
    var subscriptionsJson by remember {
        mutableStateOf(sharedPreferences.getString("subscriptions_list", defaultSubs) ?: defaultSubs)
    }
    var activeSubsJson by remember {
        mutableStateOf(sharedPreferences.getString("active_subscriptions", """["https://raw.githubusercontent.com/A-K-6/v2ray_scrapper_repo/tci_ir/subscription.txt"]""") ?: "[]")
    }

    // Target Sites State
    val defaultSites = """[
        "https://www.google.com",
        "https://www.youtube.com",
        "https://www.instagram.com",
        "https://telegram.org",
        "https://gemini.google.com",
        "https://www.openai.com"
    ]""".trimIndent()
    var targetSitesJson by remember {
        mutableStateOf(sharedPreferences.getString("target_sites_list", defaultSites) ?: defaultSites)
    }
    var selectedSite by remember {
        mutableStateOf(sharedPreferences.getString("selected_test_site", "https://www.google.com") ?: "https://www.google.com")
    }

    // Limits
    var maxDelayMs by remember {
        mutableStateOf(sharedPreferences.getInt("max_delay_ms", 3000))
    }
    var maxConfigsLimit by remember {
        mutableStateOf(sharedPreferences.getInt("max_configs_limit", 50))
    }

    // Lists for UI rendering
    var subscriptions by remember { mutableStateOf(getListFromJson(subscriptionsJson)) }
    var activeSubs by remember { mutableStateOf(getListFromJson(activeSubsJson).toSet()) }
    var targetSites by remember { mutableStateOf(getListFromJson(targetSitesJson)) }

    // Save preferences when state changes
    LaunchedEffect(coreApiUrl) { sharedPreferences.edit().putString("core_api_url", coreApiUrl).apply() }
    LaunchedEffect(subscriptions) {
        subscriptionsJson = saveListToJson(subscriptions)
        sharedPreferences.edit().putString("subscriptions_list", subscriptionsJson).apply()
    }
    LaunchedEffect(activeSubs) {
        activeSubsJson = saveListToJson(activeSubs.toList())
        sharedPreferences.edit().putString("active_subscriptions", activeSubsJson).apply()
    }
    LaunchedEffect(targetSites) {
        targetSitesJson = saveListToJson(targetSites)
        sharedPreferences.edit().putString("target_sites_list", targetSitesJson).apply()
    }
    LaunchedEffect(selectedSite) { sharedPreferences.edit().putString("selected_test_site", selectedSite).apply() }
    LaunchedEffect(maxDelayMs) { sharedPreferences.edit().putInt("max_delay_ms", maxDelayMs).apply() }
    LaunchedEffect(maxConfigsLimit) { sharedPreferences.edit().putInt("max_configs_limit", maxConfigsLimit).apply() }

    // --- Dynamic testing state ---
    var nodesList by remember { mutableStateOf(listOf<ProxyNode>()) }
    var isLoading by remember { mutableStateOf(false) }
    var statusMessage by remember { mutableStateOf("Core: Ready. Select Subscriptions and Test.") }
    var isTestingLatencies by remember { mutableStateOf(false) }
    var searchQuery by remember { mutableStateOf("") }

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
        Row(
            verticalAlignment = Alignment.CenterVertically,
            modifier = Modifier.padding(bottom = 8.dp)
        ) {
            Text(
                text = "⚡ V2Ray Updater Pro",
                fontSize = 22.sp,
                fontWeight = FontWeight.Bold,
                color = TextPrimary,
                modifier = Modifier.weight(1f)
            )
        }

        // Tab Navigation
        TabRow(
            selectedTabIndex = activeTab,
            containerColor = Color.Transparent,
            contentColor = PrimaryNeon,
            modifier = Modifier.padding(bottom = 12.dp)
        ) {
            Tab(selected = activeTab == 0, onClick = { activeTab = 0 }) {
                Column(horizontalAlignment = Alignment.CenterHorizontally, modifier = Modifier.padding(vertical = 8.dp)) {
                    Icon(Icons.Default.List, contentDescription = "Nodes", tint = if (activeTab == 0) PrimaryNeon else TextSecondary)
                    Text("Nodes", fontSize = 11.sp, fontWeight = FontWeight.Bold, color = if (activeTab == 0) TextPrimary else TextSecondary)
                }
            }
            Tab(selected = activeTab == 1, onClick = { activeTab = 1 }) {
                Column(horizontalAlignment = Alignment.CenterHorizontally, modifier = Modifier.padding(vertical = 8.dp)) {
                    Icon(Icons.Default.Refresh, contentDescription = "Subs", tint = if (activeTab == 1) PrimaryNeon else TextSecondary)
                    Text("Subs", fontSize = 11.sp, fontWeight = FontWeight.Bold, color = if (activeTab == 1) TextPrimary else TextSecondary)
                }
            }
            Tab(selected = activeTab == 2, onClick = { activeTab = 2 }) {
                Column(horizontalAlignment = Alignment.CenterHorizontally, modifier = Modifier.padding(vertical = 8.dp)) {
                    Icon(Icons.Default.Info, contentDescription = "Sites", tint = if (activeTab == 2) PrimaryNeon else TextSecondary)
                    Text("Sites", fontSize = 11.sp, fontWeight = FontWeight.Bold, color = if (activeTab == 2) TextPrimary else TextSecondary)
                }
            }
            Tab(selected = activeTab == 3, onClick = { activeTab = 3 }) {
                Column(horizontalAlignment = Alignment.CenterHorizontally, modifier = Modifier.padding(vertical = 8.dp)) {
                    Icon(Icons.Default.Settings, contentDescription = "Settings", tint = if (activeTab == 3) PrimaryNeon else TextSecondary)
                    Text("Settings", fontSize = 11.sp, fontWeight = FontWeight.Bold, color = if (activeTab == 3) TextPrimary else TextSecondary)
                }
            }
        }

        // --- Render Active Tab Content ---
        when (activeTab) {
            0 -> { // Nodes View
                // Action Buttons
                Card(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(bottom = 12.dp)
                        .border(1.dp, Color(0xFF2D2B3D), RoundedCornerShape(12.dp)),
                    colors = CardDefaults.cardColors(containerColor = CardBg)
                ) {
                    Column(modifier = Modifier.padding(12.dp)) {
                        Text(
                            text = "Site: $selectedSite",
                            fontSize = 13.sp,
                            color = TextSecondary,
                            fontWeight = FontWeight.Medium,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis
                        )
                        Spacer(modifier = Modifier.height(4.dp))
                        Text(
                            text = statusMessage,
                            fontSize = 12.sp,
                            color = SuccessGreen,
                            fontWeight = FontWeight.Bold
                        )

                        Spacer(modifier = Modifier.height(10.dp))

                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.spacedBy(8.dp)
                        ) {
                            Button(
                                onClick = {
                                    isLoading = true
                                    statusMessage = "Core is testing..."
                                    coroutineScope.launch {
                                        val subUrlsList = activeSubs.toList()
                                        val result = postCustomTestRequest(
                                            coreApiUrl = coreApiUrl,
                                            subUrls = subUrlsList,
                                            testUrl = selectedSite,
                                            maxDelayMs = maxDelayMs,
                                            limit = maxConfigsLimit
                                        )
                                        isLoading = false
                                        if (result != null) {
                                            val parsed = parseCoreTestResponse(result)
                                            nodesList = parsed
                                            statusMessage = "Core: Found ${parsed.size} working nodes!"
                                        } else {
                                            statusMessage = "Core test failed. Check settings/server status."
                                        }
                                    }
                                },
                                enabled = !isLoading && activeSubs.isNotEmpty(),
                                modifier = Modifier.weight(1.3f),
                                colors = ButtonDefaults.buttonColors(containerColor = PrimaryNeon)
                            ) {
                                if (isLoading) {
                                    CircularProgressIndicator(modifier = Modifier.size(18.dp), color = Color.White)
                                } else {
                                    Icon(Icons.Default.PlayArrow, contentDescription = "Test Core")
                                    Spacer(modifier = Modifier.width(4.dp))
                                    Text("Test on Core", fontSize = 12.sp, fontWeight = FontWeight.Bold)
                                }
                            }

                            Button(
                                onClick = {
                                    isLoading = true
                                    statusMessage = "Fetching subscriptions locally..."
                                    coroutineScope.launch {
                                        val collectedNodes = mutableListOf<ProxyNode>()
                                        activeSubs.forEach { url ->
                                            val data = fetchSubscription(url)
                                            if (data != null) {
                                                collectedNodes.addAll(parseSubscription(data))
                                            }
                                        }
                                        isLoading = false
                                        // Deduplicate
                                        val uniqueNodes = collectedNodes.distinctBy { "${it.protocol}://${it.host}:${it.port}" }
                                        nodesList = uniqueNodes
                                        statusMessage = "Fetched ${uniqueNodes.size} nodes locally. Run TCP Ping!"
                                    }
                                },
                                enabled = !isLoading && activeSubs.isNotEmpty(),
                                modifier = Modifier.weight(1f),
                                colors = ButtonDefaults.buttonColors(containerColor = Color(0xFF2D3748))
                            ) {
                                Text("Local Fetch", fontSize = 11.sp)
                            }
                        }

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
                                            testAllNodes(nodesList) {
                                                nodesList = nodesList.toList()
                                            }
                                            isTestingLatencies = false
                                            statusMessage = "TCP Ping checks finished!"
                                        }
                                    }
                                },
                                enabled = !isTestingLatencies && nodesList.isNotEmpty(),
                                colors = ButtonDefaults.buttonColors(containerColor = SecondaryNeon),
                                modifier = Modifier.weight(1f)
                            ) {
                                if (isTestingLatencies) {
                                    CircularProgressIndicator(modifier = Modifier.size(16.dp), color = Color.White)
                                } else {
                                    Text("Local TCP Ping", fontSize = 11.sp)
                                }
                            }

                            Button(
                                onClick = {
                                    val allConfigs = nodesList.joinToString("\n") { it.rawUri }
                                    onCopyToClipboard("V2Ray Configs", allConfigs)
                                },
                                enabled = nodesList.isNotEmpty(),
                                colors = ButtonDefaults.buttonColors(containerColor = Color(0xFF4A5568)),
                                modifier = Modifier.weight(1f)
                            ) {
                                Text("Copy All", fontSize = 11.sp)
                            }
                        }
                    }
                }

                // Open Clients Shortcuts
                Text("Open Client App", fontSize = 12.sp, fontWeight = FontWeight.SemiBold, color = TextSecondary, modifier = Modifier.padding(bottom = 6.dp))
                Row(
                    modifier = Modifier.fillMaxWidth().padding(bottom = 8.dp),
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    listOf(
                        Pair("v2rayNG", "ang.go.v2art"),
                        Pair("Nekobox", "moe.nb.nekobox"),
                        Pair("Sing-Box", "io.nekohasekai.singbox")
                    ).forEach { (name, pkg) ->
                        Button(
                            onClick = { onLaunchClient(pkg) },
                            colors = ButtonDefaults.buttonColors(containerColor = CardBg),
                            modifier = Modifier.weight(1f).border(1.dp, Color(0xFF3F3B5C), RoundedCornerShape(8.dp)),
                            contentPadding = PaddingValues(6.dp)
                        ) {
                            Text(name, fontSize = 11.sp, color = TextPrimary)
                        }
                    }
                }

                // Search Filter
                if (nodesList.isNotEmpty()) {
                    OutlinedTextField(
                        value = searchQuery,
                        onValueChange = { searchQuery = it },
                        placeholder = { Text("Filter configs...", color = Color.Gray, fontSize = 12.sp) },
                        singleLine = true,
                        modifier = Modifier.fillMaxWidth().padding(bottom = 8.dp),
                        colors = TextFieldDefaults.outlinedTextFieldColors(
                            focusedTextColor = TextPrimary,
                            unfocusedTextColor = TextPrimary,
                            focusedBorderColor = SecondaryNeon,
                            unfocusedBorderColor = Color(0xFF2D2B3D),
                            containerColor = Color(0xFF14131D)
                        )
                    )
                }

                // Render Nodes
                val filtered = nodesList.filter {
                    it.remark.contains(searchQuery, ignoreCase = true) ||
                            it.protocol.contains(searchQuery, ignoreCase = true)
                }

                LazyColumn(
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                    modifier = Modifier.weight(1f)
                ) {
                    items(filtered) { node ->
                        NodeCard(node = node, onCopy = { onCopyToClipboard("V2Ray Node", node.rawUri) })
                    }
                }
            }

            1 -> { // Subscriptions Management
                var newSubUrl by remember { mutableStateOf("") }

                Card(
                    modifier = Modifier.fillMaxWidth().padding(bottom = 12.dp).border(1.dp, Color(0xFF2D2B3D), RoundedCornerShape(12.dp)),
                    colors = CardDefaults.cardColors(containerColor = CardBg)
                ) {
                    Column(modifier = Modifier.padding(12.dp)) {
                        Text("Add Subscription Link", fontWeight = FontWeight.Bold, fontSize = 14.sp, color = PrimaryNeon)
                        Spacer(modifier = Modifier.height(6.dp))
                        Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            OutlinedTextField(
                                value = newSubUrl,
                                onValueChange = { newSubUrl = it },
                                placeholder = { Text("https://...", color = Color.Gray, fontSize = 12.sp) },
                                singleLine = true,
                                modifier = Modifier.weight(1f),
                                colors = TextFieldDefaults.outlinedTextFieldColors(
                                    focusedTextColor = TextPrimary,
                                    unfocusedTextColor = TextPrimary,
                                    focusedBorderColor = PrimaryNeon,
                                    containerColor = Color(0xFF14131D)
                                )
                            )
                            Button(
                                onClick = {
                                    val cleaned = newSubUrl.trim()
                                    if (cleaned.isNotBlank() && !subscriptions.contains(cleaned)) {
                                        subscriptions = subscriptions + cleaned
                                        activeSubs = activeSubs + cleaned
                                        newSubUrl = ""
                                    }
                                },
                                colors = ButtonDefaults.buttonColors(containerColor = PrimaryNeon)
                            ) {
                                Icon(Icons.Default.Add, contentDescription = "Add")
                            }
                        }
                    }
                }

                Text("Manage Subscriptions", fontSize = 13.sp, fontWeight = FontWeight.SemiBold, color = TextSecondary, modifier = Modifier.padding(bottom = 6.dp))

                LazyColumn(
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                    modifier = Modifier.weight(1f)
                ) {
                    items(subscriptions) { url ->
                        val isChecked = activeSubs.contains(url)
                        Card(
                            modifier = Modifier.fillMaxWidth().border(1.dp, Color(0xFF2D2B3D), RoundedCornerShape(8.dp)),
                            colors = CardDefaults.cardColors(containerColor = Color(0xFF161520))
                        ) {
                            Row(
                                modifier = Modifier.fillMaxWidth().padding(10.dp),
                                verticalAlignment = Alignment.CenterVertically
                            ) {
                                Checkbox(
                                    checked = isChecked,
                                    onCheckedChange = { checked ->
                                        activeSubs = if (checked) activeSubs + url else activeSubs - url
                                    },
                                    colors = CheckboxDefaults.colors(checkedColor = PrimaryNeon)
                                )
                                Spacer(modifier = Modifier.width(6.dp))
                                Column(modifier = Modifier.weight(1f)) {
                                    Text(
                                        text = url.substringAfterLast("/"),
                                        fontSize = 13.sp,
                                        fontWeight = FontWeight.SemiBold,
                                        maxLines = 1,
                                        overflow = TextOverflow.Ellipsis
                                    )
                                    Text(
                                        text = url,
                                        fontSize = 10.sp,
                                        color = TextSecondary,
                                        maxLines = 1,
                                        overflow = TextOverflow.Ellipsis,
                                        fontFamily = FontFamily.Monospace
                                    )
                                }
                                IconButton(onClick = {
                                    subscriptions = subscriptions - url
                                    activeSubs = activeSubs - url
                                }) {
                                    Icon(Icons.Default.Delete, contentDescription = "Delete", tint = DangerRed)
                                }
                            }
                        }
                    }
                }
            }

            2 -> { // Sites Management
                var newSiteUrl by remember { mutableStateOf("") }

                Card(
                    modifier = Modifier.fillMaxWidth().padding(bottom = 12.dp).border(1.dp, Color(0xFF2D2B3D), RoundedCornerShape(12.dp)),
                    colors = CardDefaults.cardColors(containerColor = CardBg)
                ) {
                    Column(modifier = Modifier.padding(12.dp)) {
                        Text("Add Website for Latency Check", fontWeight = FontWeight.Bold, fontSize = 14.sp, color = SecondaryNeon)
                        Spacer(modifier = Modifier.height(6.dp))
                        Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            OutlinedTextField(
                                value = newSiteUrl,
                                onValueChange = { newSiteUrl = it },
                                placeholder = { Text("https://...", color = Color.Gray, fontSize = 12.sp) },
                                singleLine = true,
                                modifier = Modifier.weight(1f),
                                colors = TextFieldDefaults.outlinedTextFieldColors(
                                    focusedTextColor = TextPrimary,
                                    unfocusedTextColor = TextPrimary,
                                    focusedBorderColor = SecondaryNeon,
                                    containerColor = Color(0xFF14131D)
                                )
                            )
                            Button(
                                onClick = {
                                    val cleaned = newSiteUrl.trim()
                                    if (cleaned.isNotBlank() && !targetSites.contains(cleaned)) {
                                        targetSites = targetSites + cleaned
                                        selectedSite = cleaned
                                        newSiteUrl = ""
                                    }
                                },
                                colors = ButtonDefaults.buttonColors(containerColor = SecondaryNeon)
                            ) {
                                Icon(Icons.Default.Add, contentDescription = "Add")
                            }
                        }
                    }
                }

                Text("Select Active Test Site", fontSize = 13.sp, fontWeight = FontWeight.SemiBold, color = TextSecondary, modifier = Modifier.padding(bottom = 6.dp))

                LazyColumn(
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                    modifier = Modifier.weight(1f)
                ) {
                    items(targetSites) { url ->
                        val isSelected = selectedSite == url
                        Card(
                            modifier = Modifier
                                .fillMaxWidth()
                                .border(1.dp, if (isSelected) SecondaryNeon else Color(0xFF2D2B3D), RoundedCornerShape(8.dp))
                                .clickable { selectedSite = url },
                            colors = CardDefaults.cardColors(containerColor = if (isSelected) Color(0xFF1F293D) else Color(0xFF161520))
                        ) {
                            Row(
                                modifier = Modifier.fillMaxWidth().padding(12.dp),
                                verticalAlignment = Alignment.CenterVertically
                            ) {
                                RadioButton(
                                    selected = isSelected,
                                    onClick = { selectedSite = url },
                                    colors = RadioButtonDefaults.colors(selectedColor = SecondaryNeon)
                                )
                                Spacer(modifier = Modifier.width(8.dp))
                                Text(
                                    text = url,
                                    fontSize = 13.sp,
                                    fontWeight = FontWeight.SemiBold,
                                    modifier = Modifier.weight(1f),
                                    maxLines = 1,
                                    overflow = TextOverflow.Ellipsis
                                )
                                if (targetSites.size > 1) {
                                    IconButton(onClick = {
                                        targetSites = targetSites - url
                                        if (selectedSite == url) {
                                            selectedSite = targetSites.firstOrNull() ?: ""
                                        }
                                    }) {
                                        Icon(Icons.Default.Delete, contentDescription = "Delete", tint = DangerRed)
                                    }
                                }
                            }
                        }
                    }
                }
            }

            3 -> { // Settings & Limits
                Card(
                    modifier = Modifier.fillMaxWidth().border(1.dp, Color(0xFF2D2B3D), RoundedCornerShape(12.dp)),
                    colors = CardDefaults.cardColors(containerColor = CardBg)
                ) {
                    Column(modifier = Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(14.dp)) {
                        Text("Core Service Settings", fontWeight = FontWeight.Bold, fontSize = 15.sp, color = PrimaryNeon)

                        OutlinedTextField(
                            value = coreApiUrl,
                            onValueChange = { coreApiUrl = it },
                            label = { Text("Core API Base URL", color = TextSecondary) },
                            singleLine = true,
                            modifier = Modifier.fillMaxWidth(),
                            colors = TextFieldDefaults.outlinedTextFieldColors(
                                focusedTextColor = TextPrimary,
                                unfocusedTextColor = TextPrimary,
                                focusedBorderColor = PrimaryNeon,
                                containerColor = Color(0xFF14131D)
                            )
                        )
                        Text("API on Android Emulator is http://10.0.2.2:8084\nOn same network try http://<server-ip>:8084", fontSize = 10.sp, color = TextSecondary)

                        Box(modifier = Modifier.fillMaxWidth().height(1.dp).background(Color(0xFF2D2B3D)))

                        Text("Performance & Limits", fontWeight = FontWeight.Bold, fontSize = 15.sp, color = SecondaryNeon)

                        Column {
                            Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                                Text("Max Allowed Latency", fontSize = 13.sp, color = TextPrimary)
                                Text("${maxDelayMs}ms", fontSize = 13.sp, color = SecondaryNeon, fontWeight = FontWeight.Bold)
                            }
                            Slider(
                                value = maxDelayMs.toFloat(),
                                onValueChange = { maxDelayMs = it.toInt() },
                                valueRange = 500f..8000f,
                                steps = 15,
                                colors = SliderDefaults.colors(thumbColor = SecondaryNeon, activeTrackColor = SecondaryNeon)
                            )
                        }

                        Column {
                            Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                                Text("Returned Configs Limit", fontSize = 13.sp, color = TextPrimary)
                                Text("$maxConfigsLimit nodes", fontSize = 13.sp, color = PrimaryNeon, fontWeight = FontWeight.Bold)
                            }
                            Slider(
                                value = maxConfigsLimit.toFloat(),
                                onValueChange = { maxConfigsLimit = it.toInt() },
                                valueRange = 5f..200f,
                                steps = 39,
                                colors = SliderDefaults.colors(thumbColor = PrimaryNeon, activeTrackColor = PrimaryNeon)
                            )
                        }
                    }
                }
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

            Box(
                modifier = Modifier
                    .clip(RoundedCornerShape(4.dp))
                    .background(
                        when {
                            node.localLatency == -2L -> Color(0xFF2D3748)
                            node.localLatency == -1L -> Color(0xFF5A2020)
                            node.localLatency < 250L -> Color(0xFF1C452D)
                            node.localLatency < 600L -> Color(0xFF744210)
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
                        node.localLatency < 250L -> SuccessGreen
                        node.localLatency < 600L -> WarningYellow
                        else -> DangerRed
                    }
                )
            }

            Spacer(modifier = Modifier.width(10.dp))

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

// --- Network & Parsing Helpers ---

// JSON helper conversion utilities
fun getListFromJson(jsonStr: String): List<String> {
    val list = mutableListOf<String>()
    try {
        val array = JSONArray(jsonStr)
        for (i in 0 until array.length()) {
            list.add(array.getString(i))
        }
    } catch (e: Exception) {}
    return list
}

fun saveListToJson(list: List<String>): String {
    val array = JSONArray()
    list.forEach { array.put(it) }
    return array.toString()
}

// Core service POST custom test
suspend fun postCustomTestRequest(
    coreApiUrl: String,
    subUrls: List<String>,
    testUrl: String,
    maxDelayMs: Int,
    limit: Int
): String? = withContext(Dispatchers.IO) {
    try {
        val url = URL("$coreApiUrl/subscription/test-custom")
        val conn = url.openConnection() as HttpURLConnection
        conn.requestMethod = "POST"
        conn.connectTimeout = 30000 // Server might take some time to fetch/run xray tests
        conn.readTimeout = 30000
        conn.setRequestProperty("Content-Type", "application/json; utf-8")
        conn.setRequestProperty("Accept", "application/json")
        conn.doOutput = true

        val json = JSONObject().apply {
            val urlsArray = JSONArray()
            subUrls.forEach { urlsArray.put(it) }
            put("subscription_urls", urlsArray)
            put("test_url", testUrl)
            put("max_delay_ms", maxDelayMs)
            put("limit", limit)
        }

        conn.outputStream.use { os ->
            val input = json.toString().toByteArray(Charsets.UTF_8)
            os.write(input, 0, input.size)
        }

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
            val errorReader = BufferedReader(InputStreamReader(conn.errorStream ?: conn.inputStream))
            val errorSb = java.lang.StringBuilder()
            var line: String?
            while (errorReader.readLine().also { line = it } != null) {
                errorSb.append(line).append("\n")
            }
            errorReader.close()
            android.util.Log.e("V2RayUpdater", "POST error ${conn.responseCode}: ${errorSb.toString()}")
            null
        }
    } catch (e: Exception) {
        e.printStackTrace()
        null
    }
}

fun parseCoreTestResponse(jsonStr: String): List<ProxyNode> {
    val nodes = mutableListOf<ProxyNode>()
    try {
        val root = JSONObject(jsonStr)
        val servers = root.getJSONArray("servers")
        for (i in 0 until servers.length()) {
            val server = servers.getJSONObject(i)
            val protocol = server.getString("protocol")
            val remark = server.getString("remark")
            val host = server.getString("address")
            val port = server.getInt("port")
            val delay = server.optLong("delay", -2L)
            val rawUri = server.optString("raw_uri", "")

            nodes.add(ProxyNode(
                rawUri = rawUri,
                protocol = protocol,
                remark = remark,
                host = host,
                port = port,
                localLatency = delay
            ))
        }
    } catch (e: Exception) {
        e.printStackTrace()
    }
    return nodes
}

// Local fetch backup helper
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
        val cleanData = rawData.trim()
        val decodedData = try {
            val decodedBytes = Base64.decode(cleanData, Base64.DEFAULT)
            String(decodedBytes)
        } catch (e: Exception) {
            try {
                val decodedBytes = Base64.decode(cleanData.replace("\n", "").replace("\r", ""), Base64.DEFAULT)
                String(decodedBytes)
            } catch (ex: Exception) {
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

// Local TCP socket latency testing
suspend fun testTcpLatency(host: String, port: Int, timeoutMs: Int = 3000): Long {
    return withContext(Dispatchers.IO) {
        val startTime = System.currentTimeMillis()
        try {
            Socket().use { socket ->
                socket.connect(InetSocketAddress(host, port), timeoutMs)
            }
            System.currentTimeMillis() - startTime
        } catch (e: Exception) {
            -1L
        }
    }
}

suspend fun testAllNodes(nodes: List<ProxyNode>, onProgress: (ProxyNode) -> Unit) = coroutineScope {
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
