#[path = "src/server_url.rs"]
mod server_url;

fn main() {
    println!("cargo:rerun-if-env-changed=PUBLIC_URL");
    let address = std::env::var("PUBLIC_URL")
        .expect("PUBLIC_URL is required: use make dev-desktop or set PUBLIC_URL before building");
    let server = server_url::parse(&address).unwrap_or_else(|error| panic!("{error}"));
    println!("cargo:rustc-env=SPARROW_SERVER_URL={server}");
    tauri_build::build()
}
