namespace SleepySource;

internal static class ResponsePayloads
{
    public static Dictionary<string, object?> Chat(ChatState s)
    {
        var result = new Dictionary<string, object?>(StringComparer.Ordinal)
        {
            ["settings"] = s.Settings,
            ["messages"] = s.Messages.Count == 0 ? null : s.Messages,
            ["updated_at"] = s.UpdatedAt,
            ["auth_ready"] = s.AuthReady,
            ["live_chat_connected"] = s.LiveChatConnected,
            ["webhook_subscribed"] = s.WebhookSubscribed,
            ["webhook_request_count"] = s.WebhookRequestCount,
            ["webhook_verified_count"] = s.WebhookVerifiedCount,
            ["webhook_accepted_count"] = s.WebhookAcceptedCount,
            ["webhook_rejected_count"] = s.WebhookRejectedCount,
            ["credentials_saved"] = s.CredentialsSaved,
            ["credential_storage"] = s.CredentialStorage
        };

        AddString(result, "auth_mode", s.AuthMode);
        AddString(result, "connected_channel", s.ConnectedChannel);
        AddString(result, "broadcaster_user_id", s.BroadcasterUserID);
        if (s.TokenExpiresAt != 0) result["token_expires_at"] = s.TokenExpiresAt;
        AddString(result, "live_chat_status", s.LiveChatStatus);
        AddString(result, "chatroom_id", s.ChatroomID);
        AddString(result, "webhook_subscription_id", s.WebhookSubscriptionID);
        if (s.WebhookLastEventAt != 0) result["webhook_last_event_at"] = s.WebhookLastEventAt;
        if (s.WebhookLastRequestAt != 0) result["webhook_last_request_at"] = s.WebhookLastRequestAt;
        AddString(result, "webhook_last_event_type", s.WebhookLastEventType);
        AddString(result, "webhook_last_error", s.WebhookLastError);
        AddString(result, "saved_client_id", s.SavedClientID);
        return result;
    }

    public static Dictionary<string, object?> Cloudflare(
        bool running,
        long startedAt,
        string lastError,
        string publicUrl,
        string binary,
        bool runtimeReady,
        string runtimeVersion)
    {
        var result = new Dictionary<string, object?>(StringComparer.Ordinal)
        {
            ["running"] = running,
            ["integrated"] = true,
            ["runtime_ready"] = runtimeReady,
            ["runtime_version"] = runtimeVersion,
            ["mode"] = "quick",
            ["needs_kick_setup"] = true
        };
        if (startedAt != 0) result["started_at"] = startedAt;
        AddString(result, "last_error", lastError);
        AddString(result, "public_url", publicUrl);
        if (!string.IsNullOrWhiteSpace(publicUrl)) result["webhook_url"] = publicUrl.TrimEnd('/') + "/api/chat/kick-webhook";
        AddString(result, "binary", binary);
        return result;
    }

    private static void AddString(Dictionary<string, object?> target, string key, string? value)
    {
        if (!string.IsNullOrWhiteSpace(value)) target[key] = value;
    }
}
