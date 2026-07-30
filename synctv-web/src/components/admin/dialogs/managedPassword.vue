<script setup lang="ts">
import { computed, ref } from "vue";
import { ElMessage, ElNotification } from "element-plus";
import { userStore } from "@/stores/user";
import {
  decryptManagedUserPassword,
  resetManagedUserPassword,
  revealManagedUserPassword,
  type ManagedCredentialState
} from "@/services/apis/admin";
import {
  isValidNewUserPassword,
  USER_PASSWORD_MAX_LENGTH,
  USER_PASSWORD_MIN_LENGTH,
  USER_PASSWORD_POLICY_MESSAGE
} from "@/utils/userPassword";

interface ManagedUser {
  id: string;
  username: string;
  managedCredentialState?: ManagedCredentialState;
}

const emit = defineEmits<{
  updated: [];
}>();
const { token } = userStore();
const open = ref(false);
const user = ref<ManagedUser>();
const guardianKey = ref("");
const revealedPassword = ref("");
const passwordUpdatedAt = ref(0);
const newPassword = ref("");
const confirmPassword = ref("");
const revealLoading = ref(false);
const resetLoading = ref(false);
let requestGeneration = 0;
let activeRequest: AbortController | undefined;

const stateCopy = computed(() => {
  switch (user.value?.managedCredentialState) {
    case "ready":
      return { type: "success" as const, text: "密码已同步，可使用托管密钥查看。" };
    case "key_changed":
      return {
        type: "warning" as const,
        text: "当前密钥与这条记录不一致，请重设该用户密码后再查看。"
      };
    case "key_not_configured":
      return {
        type: "error" as const,
        text: "服务端尚未配置托管密钥，请先完成 Docker 环境变量设置。"
      };
    case "unsupported_format":
      return {
        type: "error" as const,
        text: "这条托管记录来自当前网页不支持的格式，请升级前后端后再试。"
      };
    case "missing":
      return {
        type: "warning" as const,
        text: "这是升级前创建的账号，原密码无法还原。用户自行改密或由 root 重设后即可查看。"
      };
    default:
      return { type: "info" as const, text: "该账号不属于普通托管用户。" };
  }
});
const canResetPassword = computed(
  () =>
    user.value?.managedCredentialState !== "key_not_configured" &&
    user.value?.managedCredentialState !== "not_applicable"
);
const canDecryptLocally = computed(() => Boolean(globalThis.crypto?.subtle));

const openDialog = (selected: ManagedUser) => {
  if (resetLoading.value) {
    ElMessage.warning("正在重设密码，请等待操作完成");
    return;
  }
  clearSecrets();
  user.value = selected;
  open.value = true;
};
defineExpose({ openDialog });

const clearSecrets = () => {
  requestGeneration += 1;
  activeRequest?.abort();
  activeRequest = undefined;
  guardianKey.value = "";
  revealedPassword.value = "";
  passwordUpdatedAt.value = 0;
  newPassword.value = "";
  confirmPassword.value = "";
  revealLoading.value = false;
  resetLoading.value = false;
};

const beginSensitiveRequest = () => {
  activeRequest?.abort();
  const controller = new AbortController();
  activeRequest = controller;
  requestGeneration += 1;
  return { controller, generation: requestGeneration };
};

const revealPassword = async () => {
  if (!user.value || guardianKey.value.length !== 64) {
    return ElMessage.error("请输入 64 位十六进制托管密钥");
  }
  const selectedUser = user.value;
  const { controller, generation } = beginSensitiveRequest();
  try {
    revealLoading.value = true;
    const result = await revealManagedUserPassword(token.value, selectedUser.id, controller.signal);
    const password = await decryptManagedUserPassword(
      guardianKey.value,
      selectedUser.id,
      result.version,
      result.algorithm,
      result.envelope
    );
    if (generation !== requestGeneration || user.value?.id !== selectedUser.id) return;
    revealedPassword.value = password;
    passwordUpdatedAt.value = result.updatedAt;
  } catch (err: any) {
    if (controller.signal.aborted) return;
    revealedPassword.value = "";
    ElNotification({
      title: "无法查看托管密码",
      type: "error",
      message: err.response?.data?.error || err.message
    });
  } finally {
    if (generation === requestGeneration) {
      revealLoading.value = false;
      activeRequest = undefined;
    }
  }
};

const resetPassword = async () => {
  if (resetLoading.value) return;
  const selectedUser = user.value;
  if (!selectedUser || !isValidNewUserPassword(newPassword.value)) {
    return ElMessage.error(USER_PASSWORD_POLICY_MESSAGE);
  }
  if (newPassword.value !== confirmPassword.value) {
    return ElMessage.error("两次输入的密码不一致");
  }
  const passwordToSet = newPassword.value;
  activeRequest?.abort();
  activeRequest = undefined;
  requestGeneration += 1;
  try {
    resetLoading.value = true;
    await resetManagedUserPassword(token.value, selectedUser.id, passwordToSet);
    ElNotification({
      title: "密码已更新并同步托管",
      type: "success"
    });
    if (user.value?.id === selectedUser.id) {
      revealedPassword.value = passwordToSet;
      passwordUpdatedAt.value = Date.now();
      newPassword.value = "";
      confirmPassword.value = "";
      selectedUser.managedCredentialState = "ready";
    }
    emit("updated");
  } catch (err: any) {
    ElNotification({
      title: "重设密码失败",
      type: "error",
      message: err.response?.data?.error || err.message
    });
  } finally {
    resetLoading.value = false;
  }
};

const beforeClose = (done: () => void) => {
  if (resetLoading.value) {
    ElMessage.warning("正在重设密码，请等待操作完成");
    return;
  }
  done();
};
</script>

<template>
  <el-dialog
    v-model="open"
    :title="`托管密码 · ${user?.username ?? ''}`"
    :close-on-click-modal="false"
    :close-on-press-escape="!resetLoading"
    :show-close="!resetLoading"
    :before-close="beforeClose"
    class="rounded-lg dark:bg-zinc-800 w-[520px] max-sm:w-full"
    @closed="clearSecrets"
  >
    <el-alert
      :title="stateCopy.text"
      :type="stateCopy.type"
      :closable="false"
      show-icon
      class="mb-5"
    />

    <template v-if="user?.managedCredentialState === 'ready'">
      <el-alert
        v-if="!canDecryptLocally"
        title="查看密码需要通过 HTTPS 访问；重设密码不受影响。"
        type="error"
        :closable="false"
        show-icon
        class="mb-4"
      />
      <el-form v-if="canDecryptLocally" label-position="top">
        <el-form-item label="托管密钥">
          <el-input
            v-model="guardianKey"
            type="password"
            name="synctv-guardian-key"
            autocomplete="off"
            data-1p-ignore
            data-lpignore="true"
            maxlength="64"
            show-password
            placeholder="粘贴 64 位十六进制密钥"
            @keyup.enter="revealPassword"
          />
        </el-form-item>
        <el-button
          type="primary"
          :loading="revealLoading"
          :disabled="!canDecryptLocally"
          @click="revealPassword"
        >
          查看密码
        </el-button>
      </el-form>

      <div v-if="revealedPassword" class="mt-5">
        <div class="text-sm text-zinc-500 mb-2">
          当前密码，更新于 {{ new Date(passwordUpdatedAt).toLocaleString() }}
        </div>
        <div class="flex items-center gap-2">
          <el-input :model-value="revealedPassword" readonly type="password" show-password />
        </div>
      </div>
    </template>

    <template v-if="canResetPassword">
      <el-divider content-position="left">由 root 重设</el-divider>
      <el-form label-position="top">
      <el-form-item label="新密码">
        <el-input
          v-model="newPassword"
          type="password"
          :disabled="resetLoading"
          autocomplete="new-password"
          show-password
          :minlength="USER_PASSWORD_MIN_LENGTH"
          :maxlength="USER_PASSWORD_MAX_LENGTH"
        />
      </el-form-item>
      <el-form-item label="确认新密码">
        <el-input
          v-model="confirmPassword"
          type="password"
          :disabled="resetLoading"
          autocomplete="new-password"
          show-password
          :minlength="USER_PASSWORD_MIN_LENGTH"
          :maxlength="USER_PASSWORD_MAX_LENGTH"
          @keyup.enter="resetPassword"
        />
      </el-form-item>
      <div class="text-sm text-zinc-500 mb-3">{{ USER_PASSWORD_POLICY_MESSAGE }}</div>
      <el-button type="warning" :loading="resetLoading" @click="resetPassword">
        重设并同步托管
      </el-button>
      </el-form>
    </template>
  </el-dialog>
</template>
