using System.Collections.ObjectModel;
using System.Globalization;
using System.IO;
using System.Net.Http;
using System.Text.Json;
using System.Diagnostics;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Data;
using System.Windows.Threading;

namespace MyAnalyzer;

public partial class MainWindow : Window
{
    private readonly HttpClient _httpClient = new();
    private readonly ObservableCollection<RootUrlStatItem> _rootUrlItems = [];
    private readonly ObservableCollection<HistoryRecordItem> _recordItems = [];
    private readonly CollectionViewSource _recordViewSource = new();
    private readonly DispatcherTimer _settingsSaveTimer = new();
    private readonly string _settingsFilePath;

    private string? _selectedRootUrl;
    private int _currentDays = 3;
    private bool _suspendSettingsSave;

    public MainWindow()
    {
        _settingsFilePath = Path.Combine(
            Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData),
            "MyAnalyzer",
            "settings.json");

        InitializeComponent();

        RootUrlsGrid.ItemsSource = _rootUrlItems;
        _recordViewSource.Source = _recordItems;
        _recordViewSource.Filter += RecordViewSource_Filter;
        RecordsGrid.ItemsSource = _recordViewSource.View;

        _settingsSaveTimer.Interval = TimeSpan.FromMilliseconds(400);
        _settingsSaveTimer.Tick += (_, _) =>
        {
            _settingsSaveTimer.Stop();
            SaveSettingsToFile();
        };

        LoadSettingsFromFile();
        Loaded += async (_, _) => await LoadDashboardDataAsync();
        Closing += (_, _) => SaveSettingsToFile();
    }

    private async void RefreshButton_Click(object sender, RoutedEventArgs e)
    {
        await LoadDashboardDataAsync();
    }

    private async void DaysFilter_SelectionChanged(object sender, SelectionChangedEventArgs e)
    {
        if (sender is ComboBox comboBox && comboBox.SelectedValue is string raw && int.TryParse(raw, out var days))
        {
            _currentDays = days;
            if (IsLoaded)
            {
                await LoadDashboardDataAsync();
            }
        }
    }

    private void SearchTextBox_TextChanged(object sender, TextChangedEventArgs e)
    {
        _recordViewSource.View.Refresh();
    }

    private void RootUrlsGrid_SelectionChanged(object sender, SelectionChangedEventArgs e)
    {
        _selectedRootUrl = (RootUrlsGrid.SelectedItem as RootUrlStatItem)?.RootURL;
        _recordViewSource.View.Refresh();
    }

    private void ApiBaseUrlTextBox_TextChanged(object sender, TextChangedEventArgs e)
    {
        if (_suspendSettingsSave)
        {
            return;
        }

        _settingsSaveTimer.Stop();
        _settingsSaveTimer.Start();
    }

    private void RecordsGrid_MouseDoubleClick(object sender, System.Windows.Input.MouseButtonEventArgs e)
    {
        if (RecordsGrid.SelectedItem is not HistoryRecordItem item || string.IsNullOrWhiteSpace(item.URL))
        {
            return;
        }

        if (!Uri.TryCreate(item.URL, UriKind.Absolute, out var uri))
        {
            SetStatus("该记录 URL 无效，无法跳转。", "#B42318");
            return;
        }

        try
        {
            Process.Start(new ProcessStartInfo(uri.AbsoluteUri)
            {
                UseShellExecute = true
            });
            SetStatus("已打开所选记录。", "#2A7E2E");
        }
        catch (Exception ex)
        {
            SetStatus("跳转失败", "#B42318");
            MessageBox.Show(this, $"无法打开链接：{ex.Message}", "错误", MessageBoxButton.OK, MessageBoxImage.Error);
        }
    }

    private void RecordViewSource_Filter(object sender, FilterEventArgs e)
    {
        if (e.Item is not HistoryRecordItem item)
        {
            e.Accepted = false;
            return;
        }

        var keyword = SearchTextBox.Text?.Trim() ?? string.Empty;
        var keywordMatch = string.IsNullOrEmpty(keyword)
            || item.DisplayTitle.Contains(keyword, StringComparison.OrdinalIgnoreCase)
            || item.URL.Contains(keyword, StringComparison.OrdinalIgnoreCase)
            || item.RootURL.Contains(keyword, StringComparison.OrdinalIgnoreCase);

        var rootMatch = string.IsNullOrWhiteSpace(_selectedRootUrl)
            || item.RootURL.Equals(_selectedRootUrl, StringComparison.OrdinalIgnoreCase);

        e.Accepted = keywordMatch && rootMatch;
    }

    private async Task LoadDashboardDataAsync()
    {
        try
        {
            SetStatus("加载中...", "#9A6C00");
            var rootStats = await GetRootUrlStatsAsync(_currentDays);
            var records = await GetRecentRecordsAsync(300);

            _rootUrlItems.Clear();
            foreach (var item in rootStats.OrderByDescending(i => i.VisitCountTotal))
            {
                _rootUrlItems.Add(item);
            }

            _recordItems.Clear();
            foreach (var item in records.OrderByDescending(i => i.VisitedAt))
            {
                _recordItems.Add(item);
            }

            UpdateSummaryCards(rootStats, records);
            SetStatus($"已更新：{DateTime.Now:yyyy-MM-dd HH:mm:ss}", "#2A7E2E");
            _recordViewSource.View.Refresh();
        }
        catch (Exception ex)
        {
            SetStatus("加载失败", "#B42318");
            MessageBox.Show(this, $"获取数据失败：{ex.Message}", "错误", MessageBoxButton.OK, MessageBoxImage.Error);
        }
    }

    private void UpdateSummaryCards(IReadOnlyCollection<RootUrlStatItem> rootStats, IReadOnlyCollection<HistoryRecordItem> records)
    {
        RootSiteCountText.Text = rootStats.Count.ToString(CultureInfo.InvariantCulture);
        TotalVisitsText.Text = rootStats.Sum(i => i.VisitCountTotal).ToString(CultureInfo.InvariantCulture);
        RecentRecordCountText.Text = records.Count.ToString(CultureInfo.InvariantCulture);

        var latest = records.OrderByDescending(r => r.VisitedAt).FirstOrDefault();
        LastVisitText.Text = latest is null
            ? "--"
            : $"{latest.DisplayVisitedDate} {latest.DisplayVisitedTime}";
    }

    private async Task<List<RootUrlStatItem>> GetRootUrlStatsAsync(int days)
    {
        var response = await GetFromApiAsync<RootUrlStatsResponse>($"/api/history/root-urls?days={days}&limit=50");
        return response?.Items ?? [];
    }

    private async Task<List<HistoryRecordItem>> GetRecentRecordsAsync(int limit)
    {
        var response = await GetFromApiAsync<RecordListResponse>($"/api/history/recent?limit={limit}");
        return response?.Items ?? [];
    }

    private async Task<T?> GetFromApiAsync<T>(string path)
    {
        var baseUrl = (ApiBaseUrlTextBox.Text ?? string.Empty).Trim().TrimEnd('/');
        if (string.IsNullOrWhiteSpace(baseUrl))
        {
            throw new InvalidOperationException("后端地址不能为空。");
        }

        var url = $"{baseUrl}{path}";
        using var response = await _httpClient.GetAsync(url);
        response.EnsureSuccessStatusCode();

        await using var stream = await response.Content.ReadAsStreamAsync();
        return await JsonSerializer.DeserializeAsync<T>(stream, new JsonSerializerOptions
        {
            PropertyNameCaseInsensitive = true,
        });
    }

    private void SetStatus(string message, string colorHex)
    {
        StatusTextBlock.Text = message;
        StatusTextBlock.Foreground = (System.Windows.Media.Brush)new System.Windows.Media.BrushConverter().ConvertFrom(colorHex)!;
    }

    private void LoadSettingsFromFile()
    {
        try
        {
            if (!File.Exists(_settingsFilePath))
            {
                return;
            }

            var json = File.ReadAllText(_settingsFilePath);
            var settings = JsonSerializer.Deserialize<AppSettings>(json);
            if (settings is null || string.IsNullOrWhiteSpace(settings.ApiBaseUrl))
            {
                return;
            }

            _suspendSettingsSave = true;
            ApiBaseUrlTextBox.Text = settings.ApiBaseUrl.Trim();
        }
        catch
        {
            // 忽略本地配置读取失败，继续使用默认值。
        }
        finally
        {
            _suspendSettingsSave = false;
        }
    }

    private void SaveSettingsToFile()
    {
        if (_suspendSettingsSave)
        {
            return;
        }

        try
        {
            var baseUrl = (ApiBaseUrlTextBox.Text ?? string.Empty).Trim();
            var directory = Path.GetDirectoryName(_settingsFilePath);
            if (!string.IsNullOrWhiteSpace(directory))
            {
                Directory.CreateDirectory(directory);
            }

            var json = JsonSerializer.Serialize(new AppSettings
            {
                ApiBaseUrl = baseUrl
            }, new JsonSerializerOptions { WriteIndented = true });

            File.WriteAllText(_settingsFilePath, json);
        }
        catch
        {
            // 忽略保存失败，避免影响主流程。
        }
    }
}

public class RootUrlStatsResponse
{
    public List<RootUrlStatItem> Items { get; set; } = [];
}

public class RootUrlStatItem
{
    public string RootURL { get; set; } = string.Empty;
    public int VisitCountTotal { get; set; }
    public string DisplayLastVisitedAt { get; set; } = string.Empty;
    public string LatestURL { get; set; } = string.Empty;
    public string LatestTitle { get; set; } = string.Empty;
    public int RecordCount { get; set; }
}

public class RecordListResponse
{
    public List<HistoryRecordItem> Items { get; set; } = [];
}

public class HistoryRecordItem
{
    public string URL { get; set; } = string.Empty;
    public string RootURL { get; set; } = string.Empty;
    public string DisplayTitle { get; set; } = string.Empty;
    public string DisplayVisitedAt { get; set; } = string.Empty;
    public string DisplayVisitedDate { get; set; } = string.Empty;
    public string DisplayVisitedTime { get; set; } = string.Empty;
    public DateTime VisitedAt { get; set; }
}

public class AppSettings
{
    public string ApiBaseUrl { get; set; } = string.Empty;
}
