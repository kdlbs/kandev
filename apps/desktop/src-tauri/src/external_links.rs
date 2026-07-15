use url::{Host, Url};

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ExternalLinkError {
    Empty,
    Malformed,
    UnsupportedScheme,
    CredentialsNotAllowed,
    MissingDestination,
    LocalDestination,
}

impl std::fmt::Display for ExternalLinkError {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        let message = match self {
            Self::Empty => "the URL is empty",
            Self::Malformed => "the URL is malformed",
            Self::UnsupportedScheme => "the URL scheme is not allowed",
            Self::CredentialsNotAllowed => "credentials in URLs are not allowed",
            Self::MissingDestination => "the URL has no destination",
            Self::LocalDestination => "local URLs must remain inside Kandev",
        };
        formatter.write_str(message)
    }
}

pub fn validate_external_url(input: &str) -> Result<Url, ExternalLinkError> {
    if input.is_empty() || input.trim() != input {
        return Err(ExternalLinkError::Empty);
    }

    let parsed = Url::parse(input).map_err(|_| ExternalLinkError::Malformed)?;
    if !parsed.username().is_empty() || parsed.password().is_some() {
        return Err(ExternalLinkError::CredentialsNotAllowed);
    }

    match parsed.scheme() {
        "http" | "https" => validate_web_url(parsed),
        "mailto" if !parsed.path().is_empty() => Ok(parsed),
        "mailto" => Err(ExternalLinkError::MissingDestination),
        _ => Err(ExternalLinkError::UnsupportedScheme),
    }
}

fn validate_web_url(parsed: Url) -> Result<Url, ExternalLinkError> {
    let host = parsed.host().ok_or(ExternalLinkError::MissingDestination)?;
    let is_local = match host {
        Host::Domain(domain) => {
            let domain = domain.trim_end_matches('.');
            domain.eq_ignore_ascii_case("localhost")
                || domain
                    .to_ascii_lowercase()
                    .strip_suffix(".localhost")
                    .is_some()
        }
        Host::Ipv4(address) => address.is_loopback() || address.is_unspecified(),
        Host::Ipv6(address) => {
            address.is_loopback()
                || address.is_unspecified()
                || address
                    .to_ipv4_mapped()
                    .is_some_and(|mapped| mapped.is_loopback() || mapped.is_unspecified())
        }
    };
    if is_local {
        Err(ExternalLinkError::LocalDestination)
    } else {
        Ok(parsed)
    }
}

#[cfg(feature = "desktop-runtime")]
pub fn open_validated_external_url(app: &tauri::AppHandle, input: &str) -> Result<(), String> {
    use tauri_plugin_opener::OpenerExt;

    let url = validate_external_url(input).map_err(|error| error.to_string())?;
    app.opener()
        .open_url(url.as_str(), None::<&str>)
        .map_err(|error| format!("failed to open external URL: {error}"))
}

#[cfg(feature = "desktop-runtime")]
#[tauri::command]
pub fn open_external_url(
    app: tauri::AppHandle,
    backend: tauri::State<'_, crate::backend::BackendState>,
    webview: tauri::WebviewWindow,
    url: String,
) -> Result<(), String> {
    backend.require_owned_origin(&webview)?;
    open_validated_external_url(&app, &url)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn permits_external_web_and_mail_destinations() {
        assert!(validate_external_url("https://github.com/kdlbs/kandev").is_ok());
        assert!(validate_external_url("http://example.com/docs?q=desktop").is_ok());
        assert!(validate_external_url("mailto:support@example.com?subject=Kandev").is_ok());
    }

    #[test]
    fn rejects_unsupported_malformed_and_credentialed_destinations() {
        assert_eq!(
            validate_external_url("javascript:alert(1)"),
            Err(ExternalLinkError::UnsupportedScheme)
        );
        assert_eq!(
            validate_external_url("https://user:secret@example.com"),
            Err(ExternalLinkError::CredentialsNotAllowed)
        );
        assert_eq!(
            validate_external_url("not a URL"),
            Err(ExternalLinkError::Malformed)
        );
        assert_eq!(
            validate_external_url("mailto:"),
            Err(ExternalLinkError::MissingDestination)
        );
    }

    #[test]
    fn keeps_local_web_destinations_inside_kandev() {
        for input in [
            "http://localhost:8080/task",
            "http://preview.localhost:3000",
            "http://localhost.:8080",
            "http://preview.localhost.:3000",
            "http://127.0.0.1:4242",
            "http://127.9.8.7",
            "http://[::1]:4000",
            "http://[::ffff:127.0.0.1]:4000",
            "http://0.0.0.0:8080",
            "http://[::]:8080",
        ] {
            assert_eq!(
                validate_external_url(input),
                Err(ExternalLinkError::LocalDestination),
                "{input}"
            );
        }
    }
}
