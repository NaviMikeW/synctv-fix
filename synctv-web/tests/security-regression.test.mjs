import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const readSource = (relativePath) =>
  readFile(new URL(`../${relativePath}`, import.meta.url), "utf8");

test("聊天消息安全渲染且切换房间会完整隔离影厅会话", async () => {
  const [source, appSource, roomStoreSource, movieHookSource, roomHookSource] = await Promise.all([
    readSource("src/views/Cinema.vue"),
    readSource("src/App.vue"),
    readSource("src/stores/room.ts"),
    readSource("src/hooks/useMovie.ts"),
    readSource("src/hooks/useRoom.ts")
  ]);

  assert.doesNotMatch(source, /\bv-html\b/);
  assert.match(source, /\{\{\s*item\.content\s*\}\}/);
  assert.match(source, /\{\{\s*item\.time\s*\}\}/);
  assert.match(source, /chatMessages-\$\{cinemaRoomID\}/);
  assert.match(source, /removeItem\(\s*["']chatMessages-\[object Object\]["']\s*\)/);
  assert.match(
    source,
    /Legacy clients only escaped chat content[\s\S]*?sender:\s*sender\.slice\(0,\s*64\)[\s\S]*?content:\s*decodeEscapedText\(content\)/
  );
  assert.match(
    source,
    /Object records were already normalized[\s\S]*?content:\s*record\.content\.slice\(0,\s*4096\)/
  );
  assert.doesNotMatch(source, /watch\(\s*\(\)\s*=>\s*roomID\.value/);
  assert.match(appSource, /routeViewKey\s*=\s*computed\(\(\)\s*=>\s*route\.path\)/);
  assert.match(appSource, /<RouterView\s+:key="routeViewKey"\s*\/>/);
  assert.match(source, /room\.resetRoomSession\(cinemaRoomID\)/);
  assert.match(roomStoreSource, /const resetRoomSession\s*=\s*\(roomID:\s*string\)/);
  assert.match(roomStoreSource, /currentMovie\.value\s*=\s*emptyCurrentMovie\(\)/);
  assert.match(roomStoreSource, /myInfo\.value\s*=\s*undefined/);
  assert.match(roomStoreSource, /const isRoomSessionActive\s*=/);
  assert.match(movieHookSource, /isActiveRoomSession\(\)/);
  assert.match(roomHookSource, /room\.isRoomSessionActive\(roomId,\s*roomSessionID\)/);
  assert.match(source, /const cleanupWebRTC\s*=\s*\(\)\s*=>/);
  assert.match(source, /let cinemaDisposed\s*=\s*false/);
  assert.match(source, /stopMediaStream\(localStream\.value\)/);
  assert.match(source, /onBeforeUnmount\(\(\)\s*=>\s*\{[\s\S]*?cleanupWebRTC\(\)/);
});

test("登录页不再读取或写入 localStorage 密码", async () => {
  const [loginSource, mainSource] = await Promise.all([
    readSource("src/views/auth/Login.vue"),
    readSource("src/main.ts")
  ]);

  assert.doesNotMatch(loginSource, /localStorage\.(?:getItem|setItem)\(\s*["']password["']/);
  assert.doesNotMatch(loginSource, /\bsavePwd\b/);
  assert.match(mainSource, /localStorage\.removeItem\(\s*["']password["']\s*\)/);
  assert.match(loginSource, /autocomplete="current-password"/);
});

test("播放器字幕启用 HTML 转义", async () => {
  const [playerSource, subtitleSource, safeHtmlSource] = await Promise.all([
    readSource("src/components/Player.vue"),
    readSource("src/plugins/subtitle.ts"),
    readSource("src/utils/safeHtml.ts")
  ]);

  assert.match(playerSource, /subtitle:\s*\{[\s\S]*?escape:\s*true[\s\S]*?\}/);
  assert.doesNotMatch(playerSource, /escape:\s*false/);
  assert.match(safeHtmlSource, /element\.textContent\s*=\s*text/);
  assert.match(safeHtmlSource, /decodeEscapedText/);
  assert.match(safeHtmlSource, /&\(amp\|lt\|gt\|#34\|#39\);/);
  assert.match(subtitleSource, /html:\s*textToSafeHtml\(key\)/);
  assert.doesNotMatch(subtitleSource, /html:\s*key[,}]/);
});

test("通知组件使用 textContent 而不是 HTML 字符串", async () => {
  const source = await readSource("src/utils/notify.ts");

  assert.doesNotMatch(source, /\.innerHTML\b/);
  assert.match(source, /title\.textContent\s*=\s*this\.title/);
  assert.match(source, /content\.textContent\s*=\s*this\.content/);
});

test("所有新用户密码入口共享 12 到 32 位规则", async () => {
  const [registerSource, resetSource, adminSource, dialogSource, policySource] = await Promise.all([
    readSource("src/views/auth/Register.vue"),
    readSource("src/views/auth/Reset.vue"),
    readSource("src/components/admin/dialogs/newUser.vue"),
    readSource("src/components/user/dialogs/password.vue"),
    readSource("src/utils/userPassword.ts")
  ]);

  for (const source of [registerSource, resetSource, adminSource, dialogSource]) {
    assert.match(source, /@\/utils\/userPassword/);
  }
  assert.match(policySource, /USER_PASSWORD_MIN_LENGTH\s*=\s*12/);
  assert.match(policySource, /USER_PASSWORD_MAX_LENGTH\s*=\s*32/);
  assert.match(policySource, /USER_PASSWORD_PRINTABLE_ASCII_PATTERN/);
  assert.match(policySource, /isValidNewUserPassword/);
  assert.match(adminSource, /type="password"/);
  assert.doesNotMatch(adminSource, /v-model="formData\.password"\s+type="text"/);
});

test("所有动态播放器菜单名称都先转换为安全 HTML", async () => {
  const [cinemaSource, controlSource, subtitleSource] = await Promise.all([
    readSource("src/views/Cinema.vue"),
    readSource("src/plugins/control.ts"),
    readSource("src/plugins/subtitle.ts")
  ]);

  assert.doesNotMatch(cinemaSource, /html:\s*item\.name/);
  assert.match(cinemaSource, /html:\s*textToSafeHtml\(item\.name\)/);
  assert.match(controlSource, /html:\s*textToSafeHtml\(getName\(item\)\)/);
  assert.match(controlSource, /html:\s*textToSafeHtml\(option\.name\)/);
  assert.match(subtitleSource, /html:\s*textToSafeHtml\(key\)/);
});

test("托管密码只对 root 开放且密钥不写入浏览器存储", async () => {
  const [managerSource, dialogSource, apiSource] = await Promise.all([
    readSource("src/views/admin/settings/UserManager.vue"),
    readSource("src/components/admin/dialogs/managedPassword.vue"),
    readSource("src/services/apis/admin.ts")
  ]);

  assert.match(managerSource, /info\.value\?\.role\s*===\s*ROLE\.Root/);
  assert.match(managerSource, /v-if=[\s\S]*?isRoot[\s\S]*?managedCredentialState/);
  assert.match(apiSource, /["']\/api\/admin\/user\/managed-password["']/);
  assert.match(apiSource, /globalThis\.crypto\.subtle\.decrypt/);
  assert.match(apiSource, /synctv:managed-password:v1:\$\{userId\}/);
  assert.match(apiSource, /version !== 1 \|\| algorithm !== "AES-256-GCM"/);
  assert.doesNotMatch(apiSource, /\{\s*id,\s*guardianKey\s*\}/);
  assert.match(dialogSource, /guardianKey\.value\s*=\s*["']/);
  assert.match(dialogSource, /v-if="canDecryptLocally"/);
  assert.match(dialogSource, /autocomplete="off"/);
  assert.match(dialogSource, /if\s*\(resetLoading\.value\)\s*return/);
  assert.doesNotMatch(dialogSource, /CopyButton/);
  assert.doesNotMatch(dialogSource, /localStorage|sessionStorage|useStorage/);
  assert.match(dialogSource, /@closed="clearSecrets"/);
  assert.match(dialogSource, /new AbortController\(\)/);
  assert.match(dialogSource, /activeRequest\?\.abort\(\)/);
  assert.match(dialogSource, /generation !== requestGeneration/);
  assert.match(dialogSource, /:before-close="beforeClose"/);
  assert.match(dialogSource, /正在重设密码，请等待操作完成/);
  assert.match(dialogSource, /resetManagedUserPassword\(token\.value, selectedUser\.id, passwordToSet\)/);
  assert.match(dialogSource, /重设并同步托管/);
  assert.match(managerSource, /ElMessageBox\.prompt/);
  assert.match(managerSource, /降级并设置密码/);
});
