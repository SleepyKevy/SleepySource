using System.Text.Json;
using System.Text.Json.Nodes;

namespace SleepySource;

internal static class WireCompat
{
    public static CountdownState CountdownState(CountdownState state)
    {
        if (state.Fonts is { Count: 0 }) state.Fonts = null;
        if (state.Profiles is { Count: 0 }) state.Profiles = null;
        return state;
    }

    public static JsonNode AlertSettings(AlertSettings settings)
    {
        var node = ToObject(settings);
        CleanAlertSettings(node);
        return node;
    }

    public static JsonNode AlertState(AlertState state)
    {
        var node = ToObject(state);
        if (node["queue"] is JsonArray queue && queue.Count == 0)
            node["queue"] = null;
        if (node["settings"] is JsonObject settings)
            CleanAlertSettings(settings);
        return node;
    }

    public static JsonNode ChatState(ChatState state)
    {
        var node = ToObject(state);
        if (node["messages"] is JsonArray messages && messages.Count == 0)
            node["messages"] = null;

        foreach (var key in new[]
        {
            "auth_mode", "connected_channel", "broadcaster_user_id", "live_chat_status",
            "chatroom_id", "webhook_subscription_id", "webhook_last_event_type",
            "webhook_last_error", "saved_client_id"
        })
            RemoveEmptyString(node, key);

        return node;
    }

    public static JsonNode Cloudflare(object state)
    {
        var node = ToObject(state);
        foreach (var key in new[] { "last_error", "public_url", "webhook_url", "binary" })
            RemoveEmptyString(node, key);
        RemoveZero(node, "started_at");
        return node;
    }

    private static JsonObject ToObject(object value) =>
        JsonSerializer.SerializeToNode(value, AppUtil.Json) as JsonObject
        ?? throw new InvalidOperationException("SleepySource wire response is not a JSON object");

    private static void CleanAlertSettings(JsonObject settings)
    {
        if (settings["types"] is not JsonObject types) return;
        foreach (var pair in types)
        {
            if (pair.Value is not JsonObject style) continue;
            RemoveEmptyString(style, "visual_file");
            RemoveEmptyString(style, "sound_file");
        }
    }

    private static void RemoveEmptyString(JsonObject node, string key)
    {
        if (node[key] is JsonValue value && value.TryGetValue<string>(out var text) && text.Length == 0)
            node.Remove(key);
    }

    private static void RemoveZero(JsonObject node, string key)
    {
        if (node[key] is not JsonValue value) return;
        if (value.TryGetValue<long>(out var longValue) && longValue == 0)
            node.Remove(key);
        else if (value.TryGetValue<int>(out var intValue) && intValue == 0)
            node.Remove(key);
    }
}
