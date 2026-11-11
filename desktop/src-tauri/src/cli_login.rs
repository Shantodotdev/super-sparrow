//! Opens the CLI sign-in approval page from a `super-sparrow://cli-login?code=…` link.

/// The page in the server's app where a person approves a CLI sign-in.
const APPROVE_PATH: &str = "settings/cli";

/// Turns a deep link into the approval page on `server`, or `None` for any other link. Only the
/// code is carried over, and only when it looks like one, so a link cannot steer the window
/// anywhere else.
pub fn approval_page(link: &url::Url, server: &url::Url) -> Option<url::Url> {
    if link.scheme() != "super-sparrow" || link.host_str() != Some("cli-login") {
        return None;
    }
    let code = link
        .query_pairs()
        .find(|(name, _)| name == "code")
        .map(|(_, value)| value.into_owned())?;
    if !is_user_code(&code) {
        return None;
    }

    let mut page = server.join(APPROVE_PATH).ok()?;
    page.query_pairs_mut().append_pair("code", &code);
    Some(page)
}

/// Accepts the eight letters a CLI shows, with or without the dash after the fourth.
fn is_user_code(code: &str) -> bool {
    let letters: String = code.chars().filter(|c| *c != '-').collect();
    code.len() <= 9 && letters.len() == 8 && letters.chars().all(|c| c.is_ascii_alphabetic())
}

#[cfg(test)]
mod tests {
    use super::approval_page;

    fn server() -> url::Url {
        url::Url::parse("https://sparrow.example.com/").unwrap()
    }

    #[test]
    fn opens_the_approval_page_with_the_code() {
        let link = url::Url::parse("super-sparrow://cli-login?code=BCDF-GHJK").unwrap();
        assert_eq!(
            approval_page(&link, &server()).unwrap().as_str(),
            "https://sparrow.example.com/settings/cli?code=BCDF-GHJK"
        );
    }

    #[test]
    fn ignores_other_links_and_bad_codes() {
        for link in [
            "super-sparrow://other?code=BCDF-GHJK",
            "https://cli-login/?code=BCDF-GHJK",
            "super-sparrow://cli-login",
            "super-sparrow://cli-login?code=BCDF-GHJ1",
            "super-sparrow://cli-login?code=BCDF-GHJK-LMNP",
            "super-sparrow://cli-login?code=%2F%2Fevil.example.com",
        ] {
            let link = url::Url::parse(link).unwrap();
            assert!(approval_page(&link, &server()).is_none(), "{link}");
        }
    }
}
