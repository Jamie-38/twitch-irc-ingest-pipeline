using System;
using System.Net.Http;
using System.Text.Json;
using System.Threading.Tasks;
using Twitch_Ingest_Pipeline_App.Models;

namespace Twitch_Ingest_Pipeline_App.Services
{
    public class PipelineApiClient
    {
        private readonly HttpClient _httpClient;
        private readonly string _baseUrl;

        private static readonly JsonSerializerOptions JsonOptions = new()
        {
            PropertyNameCaseInsensitive = true
        };

        public PipelineApiClient(string baseUrl)
        {
            _httpClient = new HttpClient();
            _baseUrl = baseUrl.TrimEnd('/');
        }

        public async Task<string> GetHealthAsync()
        {
            return await _httpClient.GetStringAsync($"{_baseUrl}/healthz");
        }

        public async Task<string> GetReadyAsync()
        {
            return await _httpClient.GetStringAsync($"{_baseUrl}/readyz");
        }

        public async Task<ChannelsResponse> GetChannelsAsync()
        {
            string json = await _httpClient.GetStringAsync($"{_baseUrl}/channels");

            ChannelsResponse? result = JsonSerializer.Deserialize<ChannelsResponse>(json, JsonOptions);

            if (result == null)
            {
                throw new InvalidOperationException("Failed to deserialize /channels response.");
            }

            return result;
        }

        public async Task<string> JoinChannelAsync(string channel)
        {
            string safeChannel = Uri.EscapeDataString(channel);
            return await _httpClient.GetStringAsync($"{_baseUrl}/join?channel={safeChannel}");
        }

        public async Task<string> PartChannelAsync(string channel)
        {
            string safeChannel = Uri.EscapeDataString(channel);
            return await _httpClient.GetStringAsync($"{_baseUrl}/part?channel={safeChannel}");
        }
    }
}