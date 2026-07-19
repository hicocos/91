import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const installSource = readFileSync(
  new URL("../install.sh", import.meta.url),
  "utf8"
);
const releaseWorkflow = readFileSync(
  new URL("../.github/workflows/release.yml", import.meta.url),
  "utf8"
);
const releaseBuildSource = readFileSync(
  new URL("../scripts/build-release.sh", import.meta.url),
  "utf8"
);

test("installer bypasses proxy settings for local service health checks", () => {
  assert.match(
    installSource,
    /local_service_curl\(\) \{[\s\S]*?curl --disable --noproxy '\*' "\$@"/
  );
  assert.match(
    installSource,
    /if local_service_curl -fsS --connect-timeout 2 --max-time 5 "\$url"/
  );
  assert.doesNotMatch(
    installSource,
    /if curl -fsS --connect-timeout 2 --max-time 5 "\$url"/
  );
});

test("installer distinguishes readiness failures from process start failures", () => {
  assert.match(
    installSource,
    /service process is active, but its local health endpoint is unreachable/
  );
  assert.match(
    installSource,
    /listener\(s\) found on port \$port/
  );
  assert.match(
    installSource,
    /service readiness check failed; see diagnostics above/
  );
  assert.doesNotMatch(installSource, /die "service failed to start"/);
});

test("installer verifies the selected release archive before extraction", () => {
  assert.match(installSource, /download_base_url\)\/SHA256SUMS/);
  assert.match(installSource, /sha256sum -c/);
  assert.match(installSource, /checksum verification failed/);

  const verifyIndex = installSource.indexOf("sha256sum -c");
  const extractIndex = installSource.indexOf('tar -xzf "$archive"');
  assert.ok(verifyIndex >= 0 && extractIndex > verifyIndex);
});

test("installer pins self-update to an explicitly selected release", () => {
  assert.match(
    installSource,
    /INSTALL_SCRIPT_REF="\$\{INSTALL_SCRIPT_REF:-\$\{VERSION\/latest\/main\}\}"/
  );
});

test("installer snapshots and restores SQLite during upgrades", () => {
  assert.match(installSource, /backup_sqlite_database\(\)/);
  assert.match(installSource, /sqlite3[\s\S]*\.backup\(/);
  assert.match(installSource, /PRAGMA integrity_check/);
  assert.match(installSource, /restore_sqlite_database/);
  assert.match(installSource, /backup_sqlite_database "\$backup"/);
});

test("release workflow runs quality gates before publishing", () => {
  assert.match(releaseWorkflow, /npm run lint/);
  assert.match(releaseWorkflow, /npm test/);
  assert.match(releaseWorkflow, /go test \.\/\.\.\./);
  assert.match(releaseWorkflow, /go vet \.\/\.\.\./);
});

test("systemd deployment uses a dedicated user and sandbox", () => {
  assert.match(installSource, /useradd[\s\S]*video-site-91/);
  assert.match(installSource, /User=video-site-91/);
  assert.match(installSource, /Group=video-site-91/);
  assert.match(installSource, /NoNewPrivileges=true/);
  assert.match(installSource, /ProtectSystem=strict/);
  assert.match(installSource, /PrivateTmp=true/);
  assert.match(installSource, /ReadWritePaths=\$\{INSTALL_PATH\}\/data/);
});

test("release workflow publishes artifact attestations", () => {
  assert.match(releaseWorkflow, /attest-build-provenance@/);
  assert.match(releaseWorkflow, /id-token:\s*write/);
  assert.match(releaseWorkflow, /attestations:\s*write/);
});

test("release packaging emits SHA256SUMS", () => {
  assert.match(releaseBuildSource, /sha256sum[^\n]*>[^\n]*SHA256SUMS/);
});

test("release workflow refuses to overwrite an existing tag release", () => {
  assert.doesNotMatch(releaseWorkflow, /gh release delete/);
  assert.match(releaseWorkflow, /gh release view[\s\S]*exit 1/);
  assert.match(releaseWorkflow, /release\/SHA256SUMS/);
});
