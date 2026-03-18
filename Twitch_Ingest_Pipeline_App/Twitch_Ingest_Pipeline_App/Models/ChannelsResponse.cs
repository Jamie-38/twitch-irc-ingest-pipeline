using System;
using System.Collections.Generic;
using System.Text.Json.Serialization;

namespace Twitch_Ingest_Pipeline_App.Models
{
    public class ChannelsResponse
    {
        [JsonPropertyName("account")]
        public string Account { get; set; } = string.Empty;

        [JsonPropertyName("version")]
        public ulong Version { get; set; }

        [JsonPropertyName("updated_at")]
        public DateTime UpdatedAt { get; set; }

        [JsonPropertyName("channels")]
        public List<string> Channels { get; set; } = new();
    }
}