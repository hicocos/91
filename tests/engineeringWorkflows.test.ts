import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import test from "node:test";

const read = (path: string) =>
  readFileSync(new URL(`../${path}`, import.meta.url), "utf8");

test("CI gates frontend and Go quality before building", () => {
  const ci = read(".github/workflows/ci.yml");
  assert.match(ci, /npm ci/);
  assert.match(ci, /npm run lint/);
  assert.match(ci, /npm test/);
  assert.match(ci, /npm run build/);
  assert.match(ci, /npm audit/);
  assert.match(ci, /go test \.\/\.\.\./);
  assert.match(ci, /go vet \.\/\.\.\./);
  assert.match(ci, /govulncheck \.\/\.\.\./);
  assert.match(ci, /timeout-minutes:/);
  assert.match(ci, /concurrency:/);
  assert.match(ci, /permissions:[\s\S]*contents:\s*read/);
});

test("Docker publishing waits for its quality gate", () => {
  const workflow = read(".github/workflows/docker-build.yml");
  assert.match(workflow, /^\s*quality:\s*$/m);
  assert.match(workflow, /npm run lint/);
  assert.match(workflow, /npm test/);
  assert.match(workflow, /go test \.\/\.\.\./);
  assert.match(workflow, /^\s*needs:\s*quality\s*$/m);
});

test("Docker CI builds, scans vulnerabilities, and creates an SBOM", () => {
  const workflow = read(".github/workflows/docker-build.yml");
  assert.match(workflow, /docker\/build-push-action@/);
  assert.match(workflow, /trivy-action@/);
  assert.match(workflow, /anchore\/sbom-action@/);
  assert.match(workflow, /timeout-minutes:/);
  assert.match(workflow, /concurrency:/);
});

test("Dependabot covers npm, Go modules, Docker, and Actions", () => {
  assert.ok(existsSync(new URL("../.github/dependabot.yml", import.meta.url)));
  const config = read(".github/dependabot.yml");
  for (const ecosystem of ["npm", "gomod", "docker", "github-actions"]) {
    assert.match(config, new RegExp(`package-ecosystem:\\s*["']?${ecosystem}`));
  }
  assert.match(config, /directory:\s*["']?\/backend/);
});
