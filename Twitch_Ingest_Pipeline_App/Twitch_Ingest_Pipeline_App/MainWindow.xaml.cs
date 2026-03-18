using System;
using System.Diagnostics;
using System.Threading.Tasks;
using System.Windows;
using System.Windows.Threading;
using Twitch_Ingest_Pipeline_App.Models;
using Twitch_Ingest_Pipeline_App.Services;

namespace Twitch_Ingest_Pipeline_App
{
    public partial class MainWindow : Window
    {
        private readonly PipelineApiClient _apiClient;
        private readonly DispatcherTimer _pollTimer;
        private bool _isRefreshing;

        public MainWindow()
        {
            InitializeComponent();

            _apiClient = new PipelineApiClient("http://127.0.0.1:6060");

            _pollTimer = new DispatcherTimer
            {
                Interval = TimeSpan.FromSeconds(5)
            };
            _pollTimer.Tick += PollTimer_Tick;

            RefreshButton.Click += RefreshButton_Click;
            JoinButton.Click += JoinButton_Click;
            PartButton.Click += PartButton_Click;

            Debug.WriteLine("MainWindow initialized.");
            Loaded += MainWindow_Loaded;
            Closed += MainWindow_Closed;
        }

        private async void MainWindow_Loaded(object sender, RoutedEventArgs e)
        {
            AppendLog("MainWindow loaded.");
            await RefreshAllAsync();
            _pollTimer.Start();
            AppendLog("Polling started.");
        }

        private void MainWindow_Closed(object? sender, EventArgs e)
        {
            _pollTimer.Stop();
            Debug.WriteLine("Polling stopped.");
        }

        private async void PollTimer_Tick(object? sender, EventArgs e)
        {
            await RefreshAllAsync();
        }

        private async Task RefreshAllAsync()
        {
            if (_isRefreshing)
            {
                AppendLog("Refresh skipped: previous cycle still running.");
                return;
            }

            _isRefreshing = true;

            try
            {
                AppendLog("=== Refresh cycle started ===");

                string health = await _apiClient.GetHealthAsync();
                IRCHealthTextBlock.Text = health.Trim();
                AppendLog($"IRC Health: {health.Trim()}");

                string ready = await _apiClient.GetReadyAsync();
                IRCReadyTextBlock.Text = ready.Trim();
                AppendLog($"IRC Ready: {ready.Trim()}");

                ChannelsResponse channelsResponse = await _apiClient.GetChannelsAsync();
                AccountTextBlock.Text = channelsResponse.Account;
                ChannelsListBox.Items.Clear();

                if (channelsResponse.Channels.Count == 0)
                {
                    AppendLog("Channels: none");
                }
                else
                {
                    foreach (string channel in channelsResponse.Channels)
                    {
                        ChannelsListBox.Items.Add(channel);
                    }

                    AppendLog($"Channels refreshed: {channelsResponse.Channels.Count} channel(s)");
                }

                LastRefreshTextBlock.Text = DateTime.Now.ToString("HH:mm:ss");

                AppendLog("=== Refresh cycle finished ===");
                AppendLog(string.Empty);
            }
            catch (Exception ex)
            {
                AppendLog($"Refresh failed: {ex.Message}");
            }
            finally
            {
                _isRefreshing = false;
            }
        }

        private void AppendLog(string message)
        {
            string line = $"[{DateTime.Now:HH:mm:ss}] {message}";

            ActivityLogTextBox.AppendText(line + Environment.NewLine);
            ActivityLogTextBox.ScrollToEnd();

            Debug.WriteLine(line);
        }

        private async void RefreshButton_Click(object sender, RoutedEventArgs e)
        {
            AppendLog("Manual refresh requested.");
            await RefreshAllAsync();
        }

        private async void JoinButton_Click(object sender, RoutedEventArgs e)
        {
            string? channel = GetChannelInput();
            if (channel == null)
            {
                return;
            }

            try
            {
                string response = await _apiClient.JoinChannelAsync(channel);
                AppendLog($"Join requested for '{channel}': {response}");
                await RefreshAllAsync();
            }
            catch (Exception ex)
            {
                AppendLog($"Join failed for '{channel}': {ex.Message}");
            }
        }

        private async void PartButton_Click(object sender, RoutedEventArgs e)
        {
            string? channel = GetChannelInput();
            if (channel == null)
            {
                return;
            }

            try
            {
                string response = await _apiClient.PartChannelAsync(channel);
                AppendLog($"Part requested for '{channel}': {response}");
                await RefreshAllAsync();
            }
            catch (Exception ex)
            {
                AppendLog($"Part failed for '{channel}': {ex.Message}");
            }
        }

        private string? GetChannelInput()
        {
            string channel = ChannelInputTextBox.Text.Trim();

            if (string.IsNullOrWhiteSpace(channel))
            {
                AppendLog("No channel entered.");
                return null;
            }

            return channel;
        }
    }
}