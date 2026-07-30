<script lang="ts" setup>
import { computed, onMounted, ref } from "vue";
import { ElNotification, ElMessage, ElMessageBox } from "element-plus";
import { Search } from "@element-plus/icons-vue";
import { userStore } from "@/stores/user";
import {
  userListApi,
  banUserApi,
  unbanUserApi,
  addAdminApi,
  delAdminApi,
  approveUserApi,
  delUserApi,
  type ManagedCredentialState
} from "@/services/apis/admin";
import CopyButton from "@/components/CopyButton.vue";
import userRooms from "@/components/admin/dialogs/userRooms.vue";
import newUser from "@/components/admin/dialogs/newUser.vue";
import managedPassword from "@/components/admin/dialogs/managedPassword.vue";
import { ROLE, role } from "@/types/User";
import {
  isValidNewUserPassword,
  USER_PASSWORD_POLICY_MESSAGE
} from "@/utils/userPassword";

const props = defineProps<{
  title: string;
}>();

const userRoomsDialog = ref<InstanceType<typeof userRooms>>();
const newUserDialog = ref<InstanceType<typeof newUser>>();
const managedPasswordDialog = ref<InstanceType<typeof managedPassword>>();
const getRole = (rawRole: ROLE) => role[rawRole];
const roles = computed(() => {
  const v = Object.values(role);
  return v.filter((r) => r !== role[ROLE.Visitor] && r !== role[ROLE.Unknown]);
});

const { token, info } = userStore();
const isRoot = computed(() => info.value?.role === ROLE.Root);
const totalItems = ref(0);
const currentPage = ref(1);
const pageSize = ref(10);
const order = ref("desc");
const sort = ref("createdAt");
const keyword = ref("");
const search = ref("all");
const role_ = ref("");
const { state, execute: reqUserListApi, isLoading: userListLoading } = userListApi();
const getUserListApi = async () => {
  try {
    await reqUserListApi({
      headers: {
        Authorization: token.value
      },
      params: {
        page: currentPage.value,
        max: pageSize.value,
        sort: sort.value,
        order: order.value,

        role: role_.value,
        search: search.value,
        keyword: keyword.value
      }
    });
    if (state.value) {
      totalItems.value = state.value.total;
    }
  } catch (err: any) {
    console.error(err);
    ElNotification({
      title: "获取用户列表失败",
      type: "error",
      message: err.response.data.error || err.message
    });
  }
};

// 封禁 / 解封 用户
const banUser = async (id: string, is: boolean) => {
  try {
    const config = {
      headers: {
        Authorization: token.value
      },
      data: {
        id: id
      }
    };
    is ? await banUserApi().execute(config) : await unbanUserApi().execute(config);
    ElNotification({
      title: `${is ? "封禁" : "解封"}成功`,
      type: "success"
    });
    await getUserListApi();
  } catch (err: any) {
    console.error(err);
    ElNotification({
      title: "错误",
      type: "error",
      message: err.response.data.error || err.message
    });
  }
};

// 设管理
const setAdmin = async (id: string, is: boolean) => {
  try {
    if (is) {
      await addAdminApi().execute({
        headers: { Authorization: token.value },
        data: { id }
      });
    } else {
      const { value: password } = await ElMessageBox.prompt(
        "降级后账号会成为普通托管用户，请设置一个新的登录密码。",
        "取消管理员身份",
        {
          inputType: "password",
          inputPlaceholder: USER_PASSWORD_POLICY_MESSAGE,
          inputValidator: (value) =>
            isValidNewUserPassword(value) || USER_PASSWORD_POLICY_MESSAGE,
          confirmButtonText: "降级并设置密码",
          cancelButtonText: "取消"
        }
      );
      await delAdminApi().execute({
        headers: { Authorization: token.value },
        data: { id, password }
      });
    }
    ElNotification({
      title: "设置成功",
      type: "success"
    });
    await getUserListApi();
  } catch (err: any) {
    if (err === "cancel" || err === "close") return;
    console.error(err);
    ElNotification({
      title: "错误",
      type: "error",
      message: err.response.data.error || err.message
    });
  }
};

// 查看用户房间
const getUserRoom = async (id: string) => {
  userRoomsDialog.value?.openDialog(id);
};

const openManagedPassword = (selectedUser: {
  id: string;
  username: string;
  managedCredentialState?: ManagedCredentialState;
}) => {
  managedPasswordDialog.value?.openDialog(selectedUser);
};

// 允许用户注册
const { execute: approveUser, isLoading: approveLoading } = approveUserApi();
const approve = async (id: string) => {
  try {
    await approveUser({
      headers: {
        Authorization: token.value
      },
      data: {
        id: id
      }
    });
    ElNotification({
      title: "设置成功",
      type: "success"
    });
    await getUserListApi();
  } catch (err: any) {
    console.error(err);
    ElNotification({
      title: "错误",
      type: "error",
      message: err.response.data.error || err.message
    });
  }
};

// 删用户
const delUser = async (id: string) => {
  try {
    await delUserApi().execute({
      headers: {
        Authorization: token.value
      },
      data: {
        id: id
      }
    });
    ElNotification({
      title: "删除成功",
      type: "success"
    });
    await getUserListApi();
  } catch (err: any) {
    console.error(err);
    ElNotification({
      title: "错误",
      type: "error",
      message: err.response.data.error || err.message
    });
  }
};

onMounted(async () => {
  await getUserListApi();
});
</script>

<template>
  <div class="card">
    <div class="card-title flex flex-wrap justify-between items-center">
      <div>
        {{ props.title }}
      </div>

      <div>
        <el-select
          v-model="role_"
          class="max-xl:my-2 max-lg:w-full"
          placeholder="权限组"
          style="width: 90px"
          @change="getUserListApi()"
        >
          <el-option label="ALL" value="" />
          <el-option v-for="r in roles" :label="r" :value="r.toLowerCase()" />
        </el-select>
        <el-input
          class="w-fit max-lg:w-full"
          v-model="keyword"
          placeholder="搜索"
          @keyup.enter="getUserListApi()"
          required
        >
          <template #prepend>
            <el-select
              v-model="search"
              @change="getUserListApi()"
              placeholder="Select"
              style="width: 90px"
            >
              <el-option label="综合" value="all" />
              <el-option label="名称" value="name" />
              <el-option label="ID" value="roomId" />
            </el-select>
          </template>
          <template #append>
            <el-button :icon="Search" @click="getUserListApi()" />
          </template>
        </el-input>
      </div>
      <el-button class="max-xl:mt-3" type="primary" @click="newUserDialog?.openDialog">
        添加用户
      </el-button>
    </div>
    <div class="card-body">
      <el-table :data="state?.list" v-loading="userListLoading" style="width: 100%">
        <el-table-column prop="username" label="用户名" min-width="80" max-width="200" />
        <el-table-column prop="id" label="ID" min-width="120" max-width="200">
          <template #default="scope">
            <div class="flex overflow-hidden text-ellipsis max-w-[100px]">
              <span class="truncate">{{ scope.row.id }}</span>
              <CopyButton size="small" :value="scope.row.id" />
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="role" label="权限组" min-width="60" max-width="120">
          <template #default="scope">
            {{ getRole(scope.row.role) }}
          </template>
        </el-table-column>
        <el-table-column prop="roomList" label="房间列表" min-width="60" max-width="200">
          <template #default="scope">
            <el-button type="primary" plain @click="getUserRoom(scope.row.id)"> 查看 </el-button>
          </template>
        </el-table-column>
        <el-table-column prop="createdAt" label="注册时间" min-width="120" max-width="250">
          <template #default="scope">
            {{ new Date(scope.row.createdAt).toLocaleString() }}
          </template>
        </el-table-column>
        <el-table-column fixed="right" label="操作" min-width="250" max-width="350">
          <template #default="scope">
            <el-button
              v-if="
                isRoot &&
                scope.row.managedCredentialState &&
                scope.row.managedCredentialState !== 'not_applicable'
              "
              type="success"
              plain
              @click="openManagedPassword(scope.row)"
            >
              托管密码
            </el-button>

            <div v-if="scope.row.role === ROLE.Pending">
              <el-button type="success" @click="approve(scope.row.id)" :loading="approveLoading">
                通过注册
              </el-button>
              <el-button type="danger" @click="banUser(scope.row.id, true)"> 禁止注册 </el-button>
              <el-popconfirm title="你确定要删除这个用户吗？" @confirm="delUser(scope.row.id)">
                <template #reference>
                  <el-button type="danger"> 删除 </el-button>
                </template>
              </el-popconfirm>
            </div>
            <div v-else-if="scope.row.role === ROLE.Banned">
              <el-button type="warning" @click="banUser(scope.row.id, false)"> 解封 </el-button>
              <el-popconfirm title="你确定要删除这个用户吗？" @confirm="delUser(scope.row.id)">
                <template #reference>
                  <el-button type="danger"> 删除 </el-button>
                </template>
              </el-popconfirm>
            </div>
            <div v-else class="phone-button">
              <el-button type="danger" plain @click="banUser(scope.row.id, true)"> 封禁 </el-button>

              <el-button
                v-if="scope.row.role < ROLE.Admin"
                type="primary"
                @click="setAdmin(scope.row.id, true)"
              >
                设为管理
              </el-button>

              <el-button v-else type="warning" @click="setAdmin(scope.row.id, false)">
                取消管理
              </el-button>

              <el-popconfirm title="你确定要删除这个用户吗？" @confirm="delUser(scope.row.id)">
                <template #reference>
                  <el-button type="danger"> 删除 </el-button>
                </template>
              </el-popconfirm>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </div>
    <div class="card-footer flex flex-wrap justify-between overflow-hidden">
      <el-button type="success" @click="getUserListApi()" :loading="userListLoading">
        更新列表
      </el-button>

      <div class="flex flex-wrap">
        <div class="text-base mr-2 max-xl:my-2">
          排序方式：<el-select
            v-model="sort"
            class="mr-2"
            placeholder="排序方式"
            @change="getUserListApi()"
            style="width: 150px"
          >
            <el-option label="用户名" value="username" />
            <el-option label="注册时间" value="createdAt" />
          </el-select>
          <button
            class="btn btn-dense"
            @click="
              order === 'desc' ? (order = 'asc') : (order = 'desc');
              getUserListApi();
            "
          >
            {{ order === "asc" ? "👆" : "👇" }}
          </button>
        </div>

        <el-pagination
          v-model:current-page="currentPage"
          v-model:page-size="pageSize"
          :pager-count="5"
          layout="sizes, prev, pager, next, jumper"
          :total="totalItems"
          @size-change="getUserListApi()"
          @current-change="getUserListApi()"
          class="flex-wrap"
        />
      </div>
    </div>
  </div>

  <userRooms ref="userRoomsDialog" />
  <newUser ref="newUserDialog" @update-user-list="getUserListApi()" />
  <managedPassword ref="managedPasswordDialog" @updated="getUserListApi()" />
</template>

<style lang="less" scoped>
@media (max-width: 1540px) {
  .phone-button {
    .el-button {
      @apply m-0 mb-2 mr-2;
    }
  }
}
</style>
