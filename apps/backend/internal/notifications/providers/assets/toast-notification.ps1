param(
    [Parameter(Mandatory = $true)]
    [string]$Title,

    [Parameter(Mandatory = $true)]
    [string]$Message,

    [Parameter(Mandatory = $false)]
    [string]$AppName = "Kandev",

    [Parameter(Mandatory = $false)]
    [int]$TimeoutMs = 10000,

    [Parameter(Mandatory = $false)]
    [string]$IconPath = ""
)

[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
$template = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
$xml = [xml]$template.GetXml()

$textNodes = $xml.GetElementsByTagName("text")
$textNodes.Item(0).AppendChild($xml.CreateTextNode($Title)) | Out-Null
$textNodes.Item(1).AppendChild($xml.CreateTextNode($Message)) | Out-Null

if ($IconPath -and $IconPath.Trim().Length -gt 0) {
    $binding = $xml.GetElementsByTagName("binding").Item(0)
    $image = $xml.CreateElement("image")
    $image.SetAttribute("placement", "appLogoOverride") | Out-Null
    $image.SetAttribute("src", $IconPath) | Out-Null
    $binding.AppendChild($image) | Out-Null
}

$xmlDoc = New-Object Windows.Data.Xml.Dom.XmlDocument
$xmlDoc.LoadXml($xml.OuterXml)
$toast = [Windows.UI.Notifications.ToastNotification]::new($xmlDoc)
$toast.Tag = $AppName
$toast.Group = $AppName
$toast.ExpirationTime = [DateTimeOffset]::Now.AddMilliseconds($TimeoutMs)

$notifier = [Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($AppName)
$notifier.Show($toast)
