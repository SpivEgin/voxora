// VoIP Server Agent - Pure Dart Web Interface
// This file compiles to JavaScript and runs in the browser

import 'dart:html';
import 'dart:convert';
import 'dart:async';
import 'package:web_socket_channel/web_socket_channel.dart';
import 'package:http/http.dart' as http;

// Global state
WebSocketChannel? wsChannel;
String? agentId;
String? selectedCallId;
Map<String, CallInfo> calls = {};
bool connected = false;
Timer? refreshTimer;

// Main entry point
void main() {
  initializeUI();
  checkAuth();
}

// Initialize UI elements
void initializeUI() {
  // Login view
  querySelector('#login-btn')?.onClick.listen((_) => handleLogin());
  querySelector('#agent-id')?.onKeyDown.listen((e) {
    if (e.key == 'Enter') {
      e.preventDefault();
      handleLogin();
    }
  });

  // Agent dashboard view
  querySelector('#logout-btn')?.onClick.listen((_) => handleLogout());
  querySelector('#refresh-btn')?.onClick.listen((_) => refreshCalls());
  
  // Transfer buttons
  querySelector('#accept-btn')?.onClick.listen((_) => acceptTransfer());
  querySelector('#reject-btn')?.onClick.listen((_) => rejectTransfer());
  querySelector('#terminate-btn')?.onClick.listen((_) => terminateCall());

  // Keyboard shortcuts
  document.onKeyDown.listen(handleKeyDown);
}

// Check if already authenticated
void checkAuth() {
  final storedAgentId = window.localStorage['voip_agent_id'];
  if (storedAgentId != null) {
    agentId = storedAgentId;
    showAgentDashboard();
    connectWebSocket();
  } else {
    showLoginView();
  }
}

// Show login view
void showLoginView() {
  querySelector('#login-view')?.classes.remove('hidden');
  querySelector('#agent-view')?.classes.add('hidden');
  (querySelector('#agent-id') as InputElement?)?.focus();
}

// Show agent dashboard
void showAgentDashboard() {
  querySelector('#login-view')?.classes.add('hidden');
  querySelector('#agent-view')?.classes.remove('hidden');
  
  // Update agent display
  final agentDisplay = querySelector('#current-agent');
  if (agentDisplay != null) {
    agentDisplay.text = 'Agent: $agentId';
  }
}

// Handle login
void handleLogin() {
  final input = querySelector('#agent-id') as InputElement?;
  final id = input?.value?.trim();
  
  if (id == null || id.isEmpty) {
    showError('Please enter an Agent ID');
    return;
  }

  agentId = id;
  window.localStorage['voip_agent_id'] = id;
  
  showAgentDashboard();
  connectWebSocket();
}

// Handle logout
void handleLogout() {
  wsChannel?.sink.close();
  agentId = null;
  selectedCallId = null;
  calls.clear();
  window.localStorage.remove('voip_agent_id');
  showLoginView();
}

// Connect to WebSocket
void connectWebSocket() {
  // Build WebSocket URL from current host, fallback to localhost
  final host = window.location.host;
  final protocol = window.location.protocol == 'https:' ? 'wss' : 'ws';
  final wsUrl = host.isNotEmpty
      ? '$protocol://$host/ws'
      : 'ws://localhost:8080/ws';

  try {
    wsChannel = WebSocketChannel.connect(Uri.parse(wsUrl));
    connected = true;

    // Listen for messages  
    wsChannel!.stream.listen(
      handleWebSocketMessage,
      onError: (dynamic error) {
        print('WebSocket error: $error');
        connected = false;
        updateConnectionStatus();
        // Attempt reconnect after error
        Future.delayed(Duration(seconds: 5), connectWebSocket);
      },
      onDone: () {
        print('WebSocket closed');
        connected = false;
        updateConnectionStatus();
        // Attempt reconnect after disconnect
        Future.delayed(Duration(seconds: 5), connectWebSocket);
      },
    );

    // Login as agent after connection is established
    sendWebSocketMessage('agent_login', {'agent_id': agentId});

    // Subscribe to call events
    sendWebSocketMessage('subscribe_calls', {});

    updateConnectionStatus();

    // Start periodic refresh
    refreshTimer?.cancel();
    refreshTimer = Timer.periodic(Duration(seconds: 1), (_) => refreshCallDisplay());

  } catch (e) {
    // Handle synchronous connection errors (invalid URL, etc.)
    print('WebSocket connection failed: $e');
    connected = false;
    updateConnectionStatus();
    showError('Failed to connect to server. Retrying...');
    // Retry connection after delay
    Future.delayed(Duration(seconds: 5), connectWebSocket);
  }
}

// Handle WebSocket messages
void handleWebSocketMessage(dynamic message) {
  try {
    if (message is! String) {
      print('Received non-string message: $message');
      return;
    }
    
    final data = jsonDecode(message) as Map<String, dynamic>;
    final msgType = data['type'] as String?;
    
    if (msgType == null) return;
    
    final payload = data['payload'] as Map<String, dynamic>? ?? <String, dynamic>{};
    
    switch (msgType) {
      case 'active_calls':
        handleActiveCalls(payload);
        break;
      case 'call_started':
        handleCallStarted(payload);
        break;
      case 'call_ended':
        handleCallEnded(payload);
        break;
      case 'transcript_update':
        handleTranscriptUpdate(payload);
        break;
      case 'transfer_requested':
        handleTransferRequest(payload);
        break;
      case 'state_change':
        handleStateChange(payload);
        break;
      case 'error':
        handleErrorMessage(payload);
        break;
      default:
        print('Unknown message type: $msgType');
    }
  } catch (e, stackTrace) {
    print('Error handling message: $e');
    print(stackTrace);
  }
}

// Handle active calls list
void handleActiveCalls(Map<String, dynamic> payload) {
  final callsList = payload['calls'] as List<dynamic>?;
  if (callsList == null) return;
  
  final newCalls = <String, CallInfo>{};
  
  for (final callData in callsList) {
    if (callData is Map<String, dynamic>) {
      final call = CallInfo.fromJson(callData);
      newCalls[call.id] = call;
    }
  }
  
  calls = newCalls;
  updateCallList();
}

// Handle new call started
void handleCallStarted(Map<String, dynamic> payload) {
  final call = CallInfo.fromJson(payload);
  calls[call.id] = call;
  updateCallList();
  showNotification('New call from ${call.callerId}');
}

// Handle call ended
void handleCallEnded(Map<String, dynamic> payload) {
  final sessionId = payload['session_id'] as String?;
  if (sessionId != null) {
    calls.remove(sessionId);
    if (selectedCallId == sessionId) {
      selectedCallId = null;
      updateTranscript();
      updateCallInfo();
    }
    updateCallList();
  }
}

// Handle transcript update
void handleTranscriptUpdate(Map<String, dynamic> payload) {
  final sessionId = payload['session_id'] as String?;
  final speaker = payload['speaker'] as String?;
  final text = payload['text'] as String?;
  
  if (sessionId != null && calls.containsKey(sessionId)) {
    calls[sessionId]?.transcript.add(
      TranscriptEntry(speaker ?? '', text ?? '', DateTime.now())
    );
    if (selectedCallId == sessionId) {
      updateTranscript();
    }
  }
}

// Handle transfer request
void handleTransferRequest(Map<String, dynamic> payload) {
  final sessionId = payload['session_id'] as String?;
  final reason = payload['reason'] as String?;
  final callerId = payload['caller_id'] as String?;
  
  if (sessionId != null) {
    selectedCallId = sessionId;
    updateCallList();
    showTransferDialog(sessionId, callerId ?? '', reason ?? '');
  }
}

// Handle state change
void handleStateChange(Map<String, dynamic> payload) {
  final sessionId = payload['session_id'] as String?;
  final newState = payload['new_state'] as String?;
  
  if (sessionId != null && calls.containsKey(sessionId)) {
    final call = calls[sessionId];
    if (call != null) {
      call.state = newState ?? '';
    }
    if (selectedCallId == sessionId) {
      updateCallInfo();
    }
    updateCallList();
  }
}

// Handle error message from server
void handleErrorMessage(Map<String, dynamic> payload) {
  final message = payload['message'] as String?;
  showError(message ?? 'Unknown error');
}

// Send WebSocket message
void sendWebSocketMessage(String msgType, Map<String, dynamic> payload) {
  if (wsChannel != null && connected) {
    final message = jsonEncode({'type': msgType, 'payload': payload});
    wsChannel!.sink.add(message);
  }
}

// Update call list display
void updateCallList() {
  final list = querySelector('#call-list');
  if (list == null) return;
  
  list.innerHtml = '';
  
  if (calls.isEmpty) {
    final emptyDiv = DivElement()
      ..className = 'call-item empty'
      ..text = 'No active calls';
    list.append(emptyDiv);
    return;
  }
  
  calls.forEach((id, call) {
    final div = DivElement();
    div.className = 'call-item ${call.id == selectedCallId ? 'selected' : ''}';
    // Use dataset with camelCase key for data-call-id
    div.dataset['callId'] = call.id;
    
    final statusIcon = getStatusIcon(call.state);
    final duration = formatDuration(call.duration);
    
    div.innerHtml = '''
      <div class="call-header">
        <span class="status">$statusIcon</span>
        <span class="caller-id">${escapeHtml(call.callerId)}</span>
        <span class="duration">$duration</span>
      </div>
      <div class="call-details">
        <span class="intent">${escapeHtml(call.intent)}</span>
        <span class="confidence">${(call.confidence * 100).toStringAsFixed(0)}%</span>
      </div>
    ''';
    
    div.onClick.listen((_) => selectCall(call.id));
    list.append(div);
  });
}

// Select a call
void selectCall(String callId) {
  selectedCallId = callId;
  updateCallList();
  updateTranscript();
  updateCallInfo();
}

// Update transcript display
void updateTranscript() {
  final container = querySelector('#transcript');
  if (container == null) return;
  
  if (selectedCallId == null || !calls.containsKey(selectedCallId)) {
    container.innerHtml = '<div class="empty">Select a call to view transcript</div>';
    return;
  }
  
  final call = calls[selectedCallId]!;
  final html = StringBuffer();
  
  for (final entry in call.transcript) {
    final className = entry.speaker == 'caller' 
        ? 'caller' 
        : entry.speaker == 'llm' 
            ? 'llm' 
            : 'agent';
    final timeStr = formatTime(entry.timestamp);
    
    html.write('''
      <div class="transcript-entry $className">
        <div class="entry-header">
          <span class="speaker">${escapeHtml(entry.speaker)}</span>
          <span class="time">$timeStr</span>
        </div>
        <div class="text">${escapeHtml(entry.text)}</div>
      </div>
    ''');
  }
  
  container.innerHtml = html.toString();
  container.scrollTop = container.scrollHeight;
}

// Update call info display
void updateCallInfo() {
  final container = querySelector('#call-info');
  if (container == null) return;
  
  if (selectedCallId == null || !calls.containsKey(selectedCallId)) {
    container.innerHtml = '<div class="empty">Select a call to view details</div>';
    return;
  }
  
  final call = calls[selectedCallId]!;
  final confidenceClass = call.confidence < 0.6 
      ? 'low' 
      : call.confidence < 0.8 
          ? 'medium' 
          : 'high';
  
  container.innerHtml = '''
    <div class="info-row"><label>Call ID:</label><span>${escapeHtml(call.id)}</span></div>
    <div class="info-row"><label>Caller:</label><span>${escapeHtml(call.callerId)}</span></div>
    <div class="info-row"><label>State:</label><span>${escapeHtml(call.state)}</span></div>
    <div class="info-row"><label>Intent:</label><span>${escapeHtml(call.intent)}</span></div>
    <div class="info-row"><label>Confidence:</label>
      <span class="confidence $confidenceClass">${(call.confidence * 100).toStringAsFixed(1)}%</span>
    </div>
    <div class="info-row"><label>Duration:</label><span>${formatDuration(call.duration)}</span></div>
  ''';
}

// Refresh call display (update durations)
void refreshCallDisplay() {
  for (final call in calls.values) {
    call.duration++;
  }
  updateCallList();
  if (selectedCallId != null) {
    updateCallInfo();
  }
}

// Refresh calls from API
Future<void> refreshCalls() async {
  try {
    final response = await http.get(Uri.parse('/api/v1/calls'));
    if (response.statusCode == 200) {
      final data = jsonDecode(response.body) as Map<String, dynamic>;
      handleActiveCalls(data);
    }
  } catch (e) {
    print('Failed to refresh calls: $e');
  }
}

// Accept transfer
void acceptTransfer() {
  if (selectedCallId != null) {
    sendWebSocketMessage('accept_transfer', {'session_id': selectedCallId});
    hideTransferDialog();
  }
}

// Reject transfer
void rejectTransfer() {
  if (selectedCallId != null) {
    sendWebSocketMessage('reject_transfer', {'session_id': selectedCallId});
    hideTransferDialog();
  }
}

// Terminate call
void terminateCall() {
  if (selectedCallId != null) {
    sendWebSocketMessage('terminate_call', {'session_id': selectedCallId});
  }
}

// Show transfer dialog
void showTransferDialog(String sessionId, String callerId, String reason) {
  final dialog = querySelector('#transfer-dialog');
  final content = querySelector('#transfer-content');
  if (dialog == null || content == null) return;
  
  content.innerHtml = '''
    <p>Transfer request for <strong>${escapeHtml(callerId)}</strong></p>
    <p>Reason: ${escapeHtml(reason)}</p>
  ''';
  
  dialog.classes.remove('hidden');
}

// Hide transfer dialog
void hideTransferDialog() {
  querySelector('#transfer-dialog')?.classes.add('hidden');
}

// Show notification
void showNotification(String message) {
  final container = querySelector('#notifications');
  if (container == null) return;
  
  final div = DivElement()
    ..className = 'notification'
    ..text = message;
  
  container.append(div);
  
  // Auto-remove after 5 seconds
  Timer(Duration(seconds: 5), () {
    if (div.parent != null) {
      div.remove();
    }
  });
}

// Show error
void showError(String message) {
  showNotification('Error: $message');
}

// Update connection status
void updateConnectionStatus() {
  final status = querySelector('#connection-status');
  if (status == null) return;
  
  if (connected) {
    status.classes
      ..remove('disconnected')
      ..add('connected');
    status.text = 'Connected';
  } else {
    status.classes
      ..remove('connected')
      ..add('disconnected');
    status.text = 'Disconnected';
  }
}

// Handle keyboard shortcuts
void handleKeyDown(KeyboardEvent event) {
  // Use event.key instead of deprecated keyCode
  switch (event.key) {
    case 'F1':
      event.preventDefault();
      acceptTransfer();
      break;
    case 'F2':
      event.preventDefault();
      rejectTransfer();
      break;
    case 'F3':
      event.preventDefault();
      terminateCall();
      break;
    case 'Escape':
      event.preventDefault();
      // Optionally close dialogs or show help
      break;
  }
}

// Helper functions
String getStatusIcon(String state) {
  switch (state) {
    case 'INCOMING': return '📞';
    case 'LLM_ROUTING': return '🤖';
    case 'LIVE_AGENT': return '👤';
    case 'TRANSFERRING': return '⏳';
    case 'TERMINATED': return '❌';
    default: return '●';
  }
}

String formatDuration(int seconds) {
  final mins = seconds ~/ 60;
  final secs = seconds % 60;
  return '${mins.toString().padLeft(2, '0')}:${secs.toString().padLeft(2, '0')}';
}

String formatTime(DateTime time) {
  final hour = time.hour.toString().padLeft(2, '0');
  final minute = time.minute.toString().padLeft(2, '0');
  return '$hour:$minute';
}

String escapeHtml(String text) {
  if (text.isEmpty) return '';
  return text
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#x27;');
}

// Data classes
class CallInfo {
  final String id;
  String state;
  final String callerId;
  int duration;
  String intent;
  double confidence;
  final List<TranscriptEntry> transcript;
  
  CallInfo({
    required this.id,
    required this.state,
    required this.callerId,
    this.duration = 0,
    this.intent = '',
    this.confidence = 0.0,
    List<TranscriptEntry>? transcript,
  }) : transcript = transcript ?? [];
  
  factory CallInfo.fromJson(Map<String, dynamic> json) {
    return CallInfo(
      id: json['id'] as String? ?? '',
      state: json['state'] as String? ?? '',
      callerId: json['caller_id'] as String? ?? '',
      duration: (json['duration'] as num?)?.toInt() ?? 0,
      intent: json['intent'] as String? ?? '',
      confidence: (json['confidence'] as num?)?.toDouble() ?? 0.0,
    );
  }
  
  // Convert to JSON for transmission
  Map<String, dynamic> toJson() {
    return {
      'id': id,
      'state': state,
      'caller_id': callerId,
      'duration': duration,
      'intent': intent,
      'confidence': confidence,
    };
  }
}

class TranscriptEntry {
  final String speaker;
  final String text;
  final DateTime timestamp;
  
  TranscriptEntry(this.speaker, this.text, this.timestamp);
  
  Map<String, dynamic> toJson() {
    return {
      'speaker': speaker,
      'text': text,
      'timestamp': timestamp.toIso8601String(),
    };
  }
}
