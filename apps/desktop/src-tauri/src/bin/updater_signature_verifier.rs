use base64::{engine::general_purpose::STANDARD, Engine};
use minisign_verify::{PublicKey, Signature};
use std::{env, fs, path::Path};

fn main() {
    if let Err(error) = run(env::args().skip(1).collect()) {
        eprintln!("{error}");
        std::process::exit(1);
    }
}

fn run(arguments: Vec<String>) -> Result<(), String> {
    if arguments.len() < 3 || arguments.len() % 2 == 0 {
        return Err(
            "usage: updater-signature-verifier <public-key> <artifact> <signature> [...]"
                .to_string(),
        );
    }

    let public_key = decode_text(&arguments[0], "updater public key")?;
    let public_key = PublicKey::decode(&public_key)
        .map_err(|error| format!("invalid updater public key: {error}"))?;

    for pair in arguments[1..].chunks_exact(2) {
        verify_pair(&public_key, Path::new(&pair[0]), Path::new(&pair[1]))?;
    }
    Ok(())
}

fn verify_pair(
    public_key: &PublicKey,
    artifact: &Path,
    signature_path: &Path,
) -> Result<(), String> {
    let bytes = fs::read(artifact).map_err(|error| {
        format!(
            "could not read updater artifact {}: {error}",
            artifact.display()
        )
    })?;
    let encoded_signature = fs::read_to_string(signature_path).map_err(|error| {
        format!(
            "could not read updater signature {}: {error}",
            signature_path.display()
        )
    })?;
    let signature_text = decode_text(encoded_signature.trim(), "updater signature")?;
    let signature = Signature::decode(&signature_text).map_err(|error| {
        format!(
            "invalid updater signature {}: {error}",
            signature_path.display()
        )
    })?;
    public_key
        .verify(&bytes, &signature, true)
        .map_err(|error| {
            format!(
                "updater signature does not match embedded public key for {}: {error}",
                artifact.display()
            )
        })
}

fn decode_text(encoded: &str, label: &str) -> Result<String, String> {
    let bytes = STANDARD
        .decode(encoded)
        .map_err(|error| format!("invalid base64 {label}: {error}"))?;
    String::from_utf8(bytes).map_err(|error| format!("invalid UTF-8 {label}: {error}"))
}
