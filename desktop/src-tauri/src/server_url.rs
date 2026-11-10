//! Validates the deployment address before embedding it in the desktop app.

/// Requires an explicit HTTPS origin, allowing HTTP only on loopback for local development.
pub fn parse(address: &str) -> Result<url::Url, String> {
    let parsed = url::Url::parse(address.trim())
        .map_err(|_| "PUBLIC_URL must be an absolute HTTP or HTTPS URL".to_string())?;
    let local = matches!(parsed.host_str(), Some("localhost" | "127.0.0.1" | "[::1]"));
    if parsed.host_str().is_none()
        || !(parsed.scheme() == "https" || parsed.scheme() == "http" && local)
    {
        return Err("PUBLIC_URL must use HTTPS; HTTP is allowed only on loopback".into());
    }
    if !parsed.username().is_empty()
        || parsed.password().is_some()
        || parsed.path() != "/"
        || parsed.query().is_some()
        || parsed.fragment().is_some()
    {
        return Err(
            "PUBLIC_URL must be an origin without credentials, a path, query, or fragment".into(),
        );
    }
    Ok(parsed)
}

#[cfg(test)]
mod tests {
    use super::parse;

    #[test]
    fn accepts_https_and_local_development_origins() {
        for address in [
            "https://sparrow.example.com",
            "http://localhost:3000",
            "http://127.0.0.1:3000",
            "http://[::1]:3000",
        ] {
            assert!(parse(address).is_ok(), "{address}");
        }
    }

    #[test]
    fn refuses_missing_malformed_and_insecure_addresses() {
        for address in [
            "",
            " ",
            "sparrow.example.com",
            "https://",
            "http://sparrow.example.com",
            "file:///etc/passwd",
            "javascript:alert(1)",
        ] {
            assert!(parse(address).is_err(), "{address}");
        }
    }

    #[test]
    fn refuses_credentials_and_non_origin_components() {
        for address in [
            "https://user:secret@sparrow.example.com",
            "https://sparrow.example.com/issues",
            "https://sparrow.example.com/?token=secret",
            "https://sparrow.example.com/#capture",
        ] {
            assert!(parse(address).is_err(), "{address}");
        }
    }
}
