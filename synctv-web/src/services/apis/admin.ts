import { useDefineApi } from "@/stores/useDefineApi";
import axios from "axios";
import type { RoomList } from "@/types/Room";
import type { ROLE } from "@/types/User";
import type { Backend } from "@/types/Vendor";

// 获取房间设置
export const roomSettings = useDefineApi<
  {
    headers: {
      Authorization: string;
    };
  },
  {
    create_room_need_review: boolean;
    disable_create_room: boolean;
    room_must_need_pwd: boolean;
    room_ttl: number;
  }
>({
  url: "/api/admin/settings/room",
  method: "GET"
});

// 添加管理员
export const addAdminApi = useDefineApi<
  {
    headers: {
      Authorization: string;
    };
    data: {
      id: string;
    };
  },
  any
>({
  url: "/api/admin/admin/add",
  method: "POST"
});

// 取消管理员身份
export const delAdminApi = useDefineApi<
  {
    headers: {
      Authorization: string;
    };
    data: {
      id: string;
      password: string;
    };
  },
  any
>({
  url: "/api/admin/admin/delete",
  method: "POST"
});

// 获取用户列表
export const userListApi = useDefineApi<
  {
    headers: {
      Authorization: string;
    };
    params: {
      page: number;
      max: number;
      sort: string;
      order: string;

      role: string;
      search: string;
      keyword: string;
    };
  },
  {
    list: {
      id: string;
      username: string;
      role: number;
      createdAt: number;
      managedCredentialState?: ManagedCredentialState;
    }[];
    total: number;
  }
>({
  url: "/api/admin/user/list"
});

export type ManagedCredentialState =
  | "not_applicable"
  | "missing"
  | "ready"
  | "key_changed"
  | "unsupported_format"
  | "key_not_configured";

export const revealManagedUserPassword = async (
  token: string,
  id: string,
  signal: AbortSignal
) => {
  const response = await axios.post<{
    data: {
      version: number;
      algorithm: string;
      envelope: string;
      updatedAt: number;
    };
  }>(
    "/api/admin/user/managed-password",
    { id },
    { headers: { Authorization: token }, signal }
  );
  return response.data.data;
};

export const decryptManagedUserPassword = async (
  guardianKey: string,
  userId: string,
  version: number,
  algorithm: string,
  envelopeBase64: string
) => {
  if (version !== 1 || algorithm !== "AES-256-GCM") {
    throw new Error("此托管密码格式暂不受当前网页支持");
  }
  if (!/^[0-9a-fA-F]{64}$/.test(guardianKey)) {
    throw new Error("托管密钥必须是 64 位十六进制字符");
  }
  if (!globalThis.crypto?.subtle) {
    throw new Error("当前连接不支持安全的本地解密，请改用 HTTPS 访问");
  }

  const keyBytes = new Uint8Array(
    guardianKey.match(/.{2}/g)!.map((value) => Number.parseInt(value, 16))
  );
  const envelope = Uint8Array.from(atob(envelopeBase64), (value) => value.charCodeAt(0));
  const nonceLength = 12;
  if (envelope.length <= nonceLength + 16) {
    throw new Error("托管密码数据无效");
  }

  const key = await globalThis.crypto.subtle.importKey(
    "raw",
    keyBytes,
    { name: "AES-GCM" },
    false,
    ["decrypt"]
  );
  const plaintext = await globalThis.crypto.subtle.decrypt(
    {
      name: "AES-GCM",
      iv: envelope.slice(0, nonceLength),
      additionalData: new TextEncoder().encode(`synctv:managed-password:v1:${userId}`),
      tagLength: 128
    },
    key,
    envelope.slice(nonceLength)
  );
  return new TextDecoder().decode(plaintext);
};

export const resetManagedUserPassword = async (
  token: string,
  id: string,
  password: string
) => {
  await axios.post(
    "/api/admin/user/password",
    { id, password },
    { headers: { Authorization: token } }
  );
};

// 封禁用户
export const banUserApi = useDefineApi<
  {
    headers: {
      Authorization: string;
    };
    data: {
      id: string;
    };
  },
  any
>({
  url: "/api/admin/user/ban",
  method: "POST"
});

// 解封用户
export const unbanUserApi = useDefineApi<
  {
    headers: {
      Authorization: string;
    };
    data: {
      id: string;
    };
  },
  any
>({
  url: "/api/admin/user/unban",
  method: "POST"
});

// 允许用户注册
export const approveUserApi = useDefineApi<
  {
    headers: {
      Authorization: string;
    };
    data: {
      id: string;
    };
  },
  any
>({
  url: "/api/admin/user/approve",
  method: "POST"
});

// 获取用户创建的房间
export const userRoomListApi = useDefineApi<
  {
    headers: {
      Authorization: string;
    };
    params: {
      page: number;
      max: number;
      sort: string;
      order: string;
      status: string;
      search: string;
      keyword: string;
      id: string;
    };
  },
  {
    list: RoomList[] | null;
    total: number;
  }
>({
  url: "/api/admin/user/rooms",
  method: "GET"
});

// 新建用户
export const newUserApi = useDefineApi<
  {
    headers: {
      Authorization: string;
    };
    data: {
      username: string;
      password: string;
      role: ROLE;
    };
  },
  any
>({
  url: "/api/admin/user/add",
  method: "POST"
});

// 删除用户
export const delUserApi = useDefineApi<
  {
    headers: {
      Authorization: string;
    };
    data: {
      id: string;
    };
  },
  any
>({
  url: "/api/admin/user/delete",
  method: "POST"
});

// 获取所有设置
export const allSettingsApi = useDefineApi<
  {
    headers: {
      Authorization: string;
    };
  },
  Record<string, Record<string, any>>
>({
  url: "/api/admin/settings",
  method: "GET"
});

// 获取用户相关设置、
export const userSettingsApi = useDefineApi<
  {
    headers: {
      Authorization: string;
    };
  },
  {
    [key: string]: any;
  }
>({
  url: "/api/admin/settings/user",
  method: "GET"
});

// 获取房间相关设置
export const roomSettingsApi = useDefineApi<
  {
    headers: {
      Authorization: string;
    };
  },
  {
    [key: string]: any;
  }
>({
  url: "/api/admin/settings/room",
  method: "GET"
});

// 修改相关设置
export const updateSettingApi = useDefineApi<
  {
    headers: {
      Authorization: string;
    };
    data: Record<string, any>;
  },
  any
>({
  url: "/api/admin/settings",
  method: "POST"
});

// 获取房间列表
export const roomListApi = useDefineApi<
  {
    headers: {
      Authorization: string;
    };
    params: {
      page: number;
      max: number;
      sort: string;
      order: string;
      search: string;
      keyword: string;
      status: string;
    };
  },
  {
    list: RoomList[] | null;
    total: number;
  }
>({
  url: "/api/admin/room/list",
  method: "GET"
});

// 封禁房间
export const banRoomApi = useDefineApi<
  {
    headers: {
      Authorization: string;
    };
    data: {
      id: string;
    };
  },
  any
>({
  url: "/api/admin/room/ban",
  method: "POST"
});

// 解封房间
export const unbanRoomApi = useDefineApi<
  {
    headers: {
      Authorization: string;
    };
    data: {
      id: string;
    };
  },
  any
>({
  url: "/api/admin/room/unban",
  method: "POST"
});

// 允许房间创建
export const approveRoomApi = useDefineApi<
  {
    headers: {
      Authorization: string;
    };
    data: {
      id: string;
    };
  },
  any
>({
  url: "/api/admin/room/approve",
  method: "POST"
});

// 删除房间
export const delRoomApi = useDefineApi<
  {
    headers: {
      Authorization: string;
    };
    data: {
      id: string;
    };
  },
  any
>({
  url: "/api/admin/room/delete",
  method: "POST"
});

// 获取 OAuth2 设置
export const oAuth2SettingsApi = useDefineApi<
  {
    headers: {
      Authorization: string;
    };
  },
  Record<string, Record<string, any>>
>({
  url: "/api/admin/settings/oauth2",
  method: "GET"
});

// 获取指定配置组
export const assignSettingApi = useDefineApi<
  {
    headers: {
      Authorization: string;
    };
    url: string;
  },
  Record<string, Record<string, any>>
>({
  method: "GET"
});

// 视频解析管理相关 Vendor后端
// 查
export const getVendorsListApi = useDefineApi<
  {
    headers: {
      Authorization: string;
    };
    page: number;
    max: number;
  },
  {
    list: {
      backend: Backend;
      usedBy: {
        bilibili: boolean;
        bilibiliBackendName: string;
        alist: boolean;
        alistBackendName: string;
        emby: boolean;
        embyBackendName: string;
      };
      status: number;
    }[];
    total: number;
  }
>({
  url: "/api/admin/vendors",
  method: "GET"
});

// 增
export const addVendorApi = useDefineApi<
  {
    headers: {
      Authorization: string;
    };
    data: Backend;
  },
  null
>({
  url: "/api/admin/vendors/add",
  method: "POST"
});

// 改
export const editVendorApi = useDefineApi<
  {
    headers: {
      Authorization: string;
    };
    data: Backend;
  },
  null
>({
  url: "/api/admin/vendors/update",
  method: "POST"
});

// 删除
export const deleteVendorApi = useDefineApi<
  {
    headers: {
      Authorization: string;
    };
    data: {
      endpoints: string[];
    };
  },
  null
>({
  url: "/api/admin/vendors/delete",
  method: "POST"
});

// 重连
export const reconnectVendorApi = useDefineApi<
  {
    headers: {
      Authorization: string;
    };
    data: {
      endpoints: string[];
    };
  },
  null
>({
  url: "/api/admin/vendors/reconnect",
  method: "POST"
});

// 发送测试邮件
export const sendTestMailApi = useDefineApi<
  {
    headers: {
      Authorization: string;
    };
    data: {
      email?: string;
    };
  },
  {}
>({
  url: "/api/admin/email/test",
  method: "POST"
});
