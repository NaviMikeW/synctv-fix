<script setup lang="ts">
import {
  computed,
  onBeforeUnmount,
  onMounted,
  ref,
  watch,
  defineAsyncComponent,
  nextTick
} from "vue";
import type { WatchStopHandle } from "vue";
import { useWebSocket, useResizeObserver } from "@vueuse/core";
import { useRouteParams } from "@vueuse/router";
import { roomStore } from "@/stores/room";
import { ElNotification, ElMessage } from "element-plus";
import { useMovieApi } from "@/hooks/useMovie";
import { useRoomApi, useRoomPermission } from "@/hooks/useRoom";
import artplayerPluginDanmuku from "artplayer-plugin-danmuku";
import { strLengthLimit, blobToUint8Array, formatTime } from "@/utils";
import { MessageType, Message, Status, messageTypeToJSON } from "@/proto/message";
import type { options } from "@/components/Player.vue";
import RoomInfo from "@/components/cinema/RoomInfo.vue";
import MovieList from "@/components/cinema/MovieList.vue";
import MoviePush from "@/components/cinema/MoviePush.vue";
import { RoomMemberPermission } from "@/types/Room";
import artplayerPluginAss from "@/plugins/artplayer-plugin-ass";
import { newSyncPlugin } from "@/plugins/sync";
import artplayerPluginMediaControl from "@/plugins/control";
import { artplayPluginSource } from "@/plugins/source";
import { currentMovieApi } from "@/services/apis/movie";
import { userStore } from "@/stores/user";
import { roomInfoApi } from "@/services/apis/room";
import { artplayerSubtitle } from "@/plugins/subtitle";
import { sendDanmu, artplayerStreamDanmu } from "@/plugins/danmu";
import { indexStore } from "@/stores";
import { decodeEscapedText, textToSafeHtml } from "@/utils/safeHtml";

const { settings } = indexStore();

const Player = defineAsyncComponent(() => import("@/components/Player.vue"));

const { token } = userStore();
// 获取房间信息
const room = roomStore();
const roomID = useRouteParams<string>("roomId");
const cinemaRoomID = roomID.value;
const roomSessionID = room.resetRoomSession(cinemaRoomID);
let cinemaDisposed = false;
const isCinemaSessionActive = () =>
  !cinemaDisposed && room.isRoomSessionActive(cinemaRoomID, roomSessionID);

interface ChatMessage {
  sender: string;
  content: string;
  time: string;
}

const MAX_MESSAGE_COUNT = 64;
const chatStorageKey = `chatMessages-${cinemaRoomID}`;

const watchers: WatchStopHandle[] = [];
onBeforeUnmount(() => {
  cinemaDisposed = true;
  watchers.forEach((w) => w());
  room.endRoomSession(cinemaRoomID, roomSessionID);
});

const { getMovies, getCurrentMovie, isLoadingCurrent } = useMovieApi(
  token.value,
  cinemaRoomID,
  roomSessionID
);
const { getMyInfo, myInfo } = useRoomApi();
const { state: roomInfo, execute: reqRoomInfoApi } = roomInfoApi();
const { hasMemberPermission } = useRoomPermission();

let player: Artplayer | undefined;

const wsProtocol = location.protocol === "https:" ? "wss:" : "ws:";
const { status, data, send, open } = useWebSocket(
  `${wsProtocol}//${window.location.host}/api/room/ws?roomId=${cinemaRoomID}`,
  {
    ...(token.value ? { protocols: [token.value] } : {}),
    autoReconnect: {
      retries: 3,
      delay: 1000,
      onFailed() {
        ElMessage.error("Websocket 自动重连失败！");
      }
    },
    autoClose: true,
    immediate: false
  }
);

const sendElement = (msg: Message) => {
  if (!msg.timestamp) {
    msg.timestamp = Date.now();
  }
  console.groupCollapsed("Ws Send");
  console.log(messageTypeToJSON(msg.type));
  console.log(msg);
  console.groupEnd();
  return send(Message.encode(msg).finish() as any, false);
};

// 消息列表
const chatMsgList = ref<ChatMessage[]>([]);

const parseLegacyChatMessage = (message: string): ChatMessage => {
  const timeMatch = message.match(/\s*<small>\[(\d{2}:\d{2}:\d{2})\]<\/small>\s*$/i);
  const text = timeMatch ? message.slice(0, timeMatch.index).trimEnd() : message;
  const colonIndex = text.indexOf(": ");
  const fullWidthColonIndex = text.indexOf("：");
  const separatorIndexes = [colonIndex, fullWidthColonIndex].filter((index) => index > 0);
  const separatorIndex = separatorIndexes.length > 0 ? Math.min(...separatorIndexes) : -1;
  const separatorLength = separatorIndex === colonIndex ? 2 : 1;
  const sender = separatorIndex > 0 ? text.slice(0, separatorIndex) : "系统";
  const content = separatorIndex > 0 ? text.slice(separatorIndex + separatorLength) : text;

  return {
    // Legacy clients only escaped chat content; the sender was stored as raw
    // text and must not be entity-decoded.
    sender: sender.slice(0, 64),
    content: decodeEscapedText(content).slice(0, 4096),
    time: timeMatch?.[1] ?? ""
  };
};

const normalizeStoredChatMessage = (message: unknown): ChatMessage | undefined => {
  if (typeof message === "string") {
    return parseLegacyChatMessage(message);
  }
  if (!message || typeof message !== "object") {
    return;
  }

  const record = message as Record<string, unknown>;
  if (typeof record.content !== "string") {
    return;
  }

  return {
    sender: typeof record.sender === "string" ? record.sender.slice(0, 64) : "系统",
    // Object records were already normalized before being persisted. Decoding
    // again on every reload would corrupt literal text such as "&lt;".
    content: record.content.slice(0, 4096),
    time: typeof record.time === "string" ? record.time.slice(0, 32) : ""
  };
};

const persistChatMessages = () => {
  try {
    sessionStorage.setItem(chatStorageKey, JSON.stringify(chatMsgList.value));
  } catch (error) {
    console.warn("保存聊天记录失败", error);
  }
};

const loadChatMessages = () => {
  const currentKey = chatStorageKey;
  chatMsgList.value = [];

  try {
    // 旧键没有房间 ID，无法安全判断记录属于哪个房间。
    sessionStorage.removeItem("chatMessages-[object Object]");
    const storedMessages = sessionStorage.getItem(currentKey);
    if (!storedMessages) {
      return;
    }

    const parsed = JSON.parse(storedMessages);
    if (!Array.isArray(parsed)) {
      return;
    }

    chatMsgList.value = parsed
      .map(normalizeStoredChatMessage)
      .filter((message): message is ChatMessage => Boolean(message))
      .slice(-MAX_MESSAGE_COUNT);
  } catch (error) {
    console.warn("读取聊天记录失败", error);
  }
};

const sendChatText = (msg: string, onSuccess?: () => any, onFailed?: () => any) => {
  if (msg.length === 0) {
    ElMessage({
      message: "发送的消息不能为空",
      type: "warning"
    });
    if (onFailed) onFailed();
    return;
  }

  strLengthLimit(msg, 4096);
  sendElement(
    Message.create({
      type: MessageType.CHAT,
      chatContent: msg
    })
  );
  if (onSuccess) onSuccess();
};

const sendChatMsg_ = ref("");
const onSendSuccess = () => {
  sendChatMsg_.value = "";
};

const sendMsg = (msg: ChatMessage) => {
  chatMsgList.value.push(msg);
  // 如果超过聊天记录最大长度，则从前面开始删除多余的消息
  nextTick(() => {
    if (chatMsgList.value.length > MAX_MESSAGE_COUNT) {
      chatMsgList.value.splice(0, chatMsgList.value.length - MAX_MESSAGE_COUNT);
    }
    persistChatMessages();
  });

  // 确保聊天区域滚动到底部
  nextTick(() => {
    if (chatArea.value) chatArea.value.scrollTop = chatArea.value.scrollHeight;
  });
};

const sendSystemMsg = (message: string) => {
  const parsed = parseLegacyChatMessage(message);
  sendMsg({
    ...parsed,
    time: parsed.time || formatTime(new Date())
  });
};

const playerOption = computed<options>(() => {
  if (!room.currentMovie.base!.url) {
    return {
      url: ""
    };
  }
  let option: options = {
    url: room.currentMovie.base!.url,
    type: room.currentMovie.base!.type,
    isLive: room.currentMovie.base!.live,
    headers: room.currentMovie.base!.headers,
    p2pZone: settings?.p2pZone,
    plugins: [
      // 弹幕
      artplayerPluginDanmuku({
        danmuku: room.currentMovie.base!.danmu || [],
        speed: 8,
        async beforeEmit(danmu: any) {
          if (danmu.direct) {
            return true;
          }
          sendChatText(danmu.text);
          return false;
        }
      }),
      // WARN: room.currentStatus 变了会导致重载
      newSyncPlugin(sendElement, room.currentStatus, () => room.currentExpireId),
      artplayerPluginMediaControl()
    ]
  };

  if (room.currentMovie.base!.moreSources) {
    const obj = room.currentMovie.base!.moreSources || [];
    option.plugins!.push(
      artplayPluginSource([
        {
          url: option.url,
          html: "默认",
          type: option.type || ""
        },
        ...obj.map((item) => ({
          url: item.url,
          html: textToSafeHtml(item.name),
          type: item.type
        }))
      ])
    );
  }

  if (room.currentMovie.base!.subtitles) {
    let defaultUrl;
    let useAssPlugin = false;

    // deep copy
    const subtitle = Object.assign({}, room.currentMovie.base!.subtitles);

    for (let key in subtitle) {
      if (subtitle[key].type === "ass") {
        useAssPlugin = true;
        defaultUrl = subtitle[key].url;
        break;
      }
    }

    option.plugins!.push(artplayerSubtitle(subtitle));
    // return;
    useAssPlugin &&
      option.plugins!.push(
        artplayerPluginAss({
          // debug: true,
          subUrl: defaultUrl
        })
      );
  }

  if (room.currentMovie.base!.streamDanmu) {
    option.plugins!.push(artplayerStreamDanmu(room.currentMovie.base!.streamDanmu));
  }

  return option;
});

const { state: currentMovie, execute: reqCurrentMovieApi } = currentMovieApi();
const updateSources = async () => {
  try {
    await reqCurrentMovieApi({
      headers: { Authorization: token.value, "X-Room-Id": cinemaRoomID }
    });
    if (!currentMovie.value || !isCinemaSessionActive()) return;
    if (currentMovie.value.movie.base.url.startsWith("/")) {
      currentMovie.value.movie.base.url = `${window.location.origin}${currentMovie.value.movie.base.url}`;
    }
    if (
      currentMovie.value.movie.base.moreSources &&
      currentMovie.value.movie.base.moreSources.length > 0
    ) {
      for (let i = 0; i < currentMovie.value.movie.base.moreSources.length; i++) {
        if (currentMovie.value.movie.base.moreSources[i].url.startsWith("/")) {
          currentMovie.value.movie.base.moreSources[i].url =
            `${window.location.origin}${currentMovie.value.movie.base.moreSources[i].url}`;
        }
      }
    }
    if (!player) return;
    room.currentExpireId = currentMovie.value.expireId;
    const moreSources = currentMovie.value.movie.base.moreSources || [];
    player.plugins["source"].updateSources([
      {
        url: currentMovie.value.movie.base.url,
        html: "默认",
        type: currentMovie.value.movie.base.type || ""
      },
      ...moreSources.map((item) => ({
        url: item.url,
        html: textToSafeHtml(item.name),
        type: item.type
      }))
    ]);
  } catch (err: any) {
    console.log(err);
    ElNotification({
      title: "获取影片列表失败",
      message: err.response.data.error || err.message,
      type: "error"
    });
  }
};

const getPlayerInstance = (art: Artplayer) => {
  player = art;
  listenPlayerType(player);
};

const playType = ref<string | undefined>();

const listenPlayerType = (player: Artplayer) => {
  player.once("ready", () => {
    playType.value = player?.option.type;
    player.on("restart", () => {
      playType.value = player?.option.type;
    });
    player.on("destroy", () => {
      playType.value = undefined;
    });
  });
};

const setPlayerStatus = (status: Status) => {
  if (!player) return;
  player.plugins["syncPlugin"].setAndNoPublishStatus(status);
};

// key: userId:connId
const peerConnections = ref<{ [key: string]: RTCPeerConnection }>({});
const localStream = ref<MediaStream | undefined>(undefined);
let remoteAudioElements: { [key: string]: HTMLAudioElement } = {};
const stopMediaStream = (stream: MediaStream | undefined) => {
  stream?.getTracks().forEach((track) => track.stop());
};

const peerConnectionsLengthWithUserId = computed(() => {
  const userIdSet = new Set(Object.keys(peerConnections.value).map((key) => key.split(":")[0]));
  return userIdSet.size;
});

// 音频设备列表
const audioInputDevices = ref<MediaDeviceInfo[]>([]);
const audioOutputDevices = ref<MediaDeviceInfo[]>([]);
const selectedAudioInput = ref("");
const selectedAudioOutput = ref("");

const outputVolume = ref(1.0); // 扬声器音量
const isMuted = ref(false); // 麦克风静音状态

// 获取音频设备列表
const getAudioDevices = async () => {
  const devices = await navigator.mediaDevices.enumerateDevices();
  audioInputDevices.value = devices.filter((device) => device.kind === "audioinput");
  audioOutputDevices.value = devices.filter((device) => device.kind === "audiooutput");
};

// 切换麦克风静音状态
const toggleMute = () => {
  if (!localStream.value) return;
  isMuted.value = !isMuted.value;
  localStream.value.getAudioTracks().forEach((track) => {
    track.enabled = !isMuted.value;
  });
};

// 切换麦克风
const switchMicrophone = async () => {
  if (!localStream.value) return;

  const previousStream = localStream.value;
  let newStream: MediaStream | undefined;
  try {
    newStream = await navigator.mediaDevices.getUserMedia({
      audio: {
        deviceId: selectedAudioInput.value,
        echoCancellation: true,
        noiseSuppression: true,
        autoGainControl: true
      }
    });
    if (!newStream) return;

    if (!isCinemaSessionActive() || localStream.value !== previousStream) {
      stopMediaStream(newStream);
      return;
    }

    // 替换所有PeerConnection中的轨道
    const [audioTrack] = newStream.getTracks();
    audioTrack.enabled = !isMuted.value; // 保持当前的静音状态

    for (const pc of Object.values(peerConnections.value)) {
      const sender = pc.getSenders().find((s: any) => s.track?.kind === "audio");
      if (sender) {
        await sender.replaceTrack(audioTrack);
        if (!isCinemaSessionActive() || localStream.value !== previousStream) {
          stopMediaStream(newStream);
          return;
        }
      }
    }

    stopMediaStream(previousStream);
    localStream.value = newStream;
  } catch (err) {
    stopMediaStream(newStream);
    if (!isCinemaSessionActive()) return;
    ElMessage.error(`切换麦克风失败: ${err}`);
  }
};

// 切换扬声器
const switchSpeaker = async () => {
  try {
    for (const audio of Object.values(remoteAudioElements)) {
      if ("setSinkId" in audio) {
        await (audio as any).setSinkId(selectedAudioOutput.value);
        audio.volume = outputVolume.value;
      }
    }
  } catch (err) {
    ElMessage.error(`切换扬声器失败: ${err}`);
  }
};

// 调整扬声器音量
const adjustOutputVolume = () => {
  Object.values(remoteAudioElements).forEach((audio) => {
    audio.volume = outputVolume.value;
  });
};

const joinWebRTC = async () => {
  let newStream: MediaStream | undefined;
  try {
    await getAudioDevices();
    if (!isCinemaSessionActive()) return;

    newStream = await navigator.mediaDevices.getUserMedia({
      audio: selectedAudioInput.value ? { deviceId: selectedAudioInput.value } : true
    });
    if (!newStream) return;
    if (!isCinemaSessionActive()) {
      stopMediaStream(newStream);
      return;
    }
    localStream.value = newStream;
  } catch (err) {
    stopMediaStream(newStream);
    if (!isCinemaSessionActive()) return;
    ElMessage.error(`获取媒体流失败！${err}`);
    return;
  }
  await sendElement(
    Message.create({
      type: MessageType.WEBRTC_JOIN
    })
  );
};

const cleanupWebRTC = () => {
  for (const pc of Object.values(peerConnections.value)) {
    pc.onicecandidate = null;
    pc.ontrack = null;
    pc.close();
  }
  peerConnections.value = {};

  stopMediaStream(localStream.value);
  localStream.value = undefined;

  for (const audio of Object.values(remoteAudioElements)) {
    audio.onended = null;
    audio.pause();
    if (audio.srcObject instanceof MediaStream) {
      audio.srcObject.getTracks().forEach((track) => track.stop());
    }
    audio.srcObject = null;
    audio.remove();
  }
  remoteAudioElements = {};
};

const exitWebRTC = async () => {
  try {
    await sendElement(
      Message.create({
        type: MessageType.WEBRTC_LEAVE
      })
    );
  } finally {
    cleanupWebRTC();
  }
};

const handleWebrtcJoin = async (msg: Message) => {
  const pc = createPeerConnection(msg.webrtcData!.from);
  const offer = await pc.createOffer();
  await pc.setLocalDescription(offer);
  sendElement(
    Message.create({
      type: MessageType.WEBRTC_OFFER,
      webrtcData: {
        data: JSON.stringify(offer),
        to: msg.webrtcData!.from
      }
    })
  );
};

const handleWebrtcLeave = async (msg: Message) => {
  closePeerConnection(msg.webrtcData!.from);
};

const closePeerConnection = (id: string) => {
  const pc = peerConnections.value[id];
  if (pc) {
    pc.close();
    delete peerConnections.value[id];
  }
  const remoteAudio = remoteAudioElements[id];
  if (remoteAudio) {
    remoteAudio.onended = null;
    remoteAudio.pause();
    remoteAudio.srcObject = null;
    remoteAudio.remove();
    delete remoteAudioElements[id];
  }
};

const handleWebrtcOffer = async (msg: Message) => {
  const data = JSON.parse(msg.webrtcData!.data);
  const pc = createPeerConnection(msg.webrtcData!.from);
  await pc.setRemoteDescription(new RTCSessionDescription(data));
  const answer = await pc.createAnswer();
  await pc.setLocalDescription(answer);
  // 将Answer发送给发送Offer的用户
  sendElement(
    Message.create({
      type: MessageType.WEBRTC_ANSWER,
      webrtcData: {
        data: JSON.stringify(answer),
        to: msg.webrtcData!.from
      }
    })
  );
};

const createPeerConnection = (id: string) => {
  const pc = new RTCPeerConnection({
    iceServers: [{ urls: "stun:stun.l.google.com:19302" }],
    iceCandidatePoolSize: 10
  });
  pc.onicecandidate = (event) => {
    if (event.candidate) {
      sendElement(
        Message.create({
          type: MessageType.WEBRTC_ICE_CANDIDATE,
          webrtcData: {
            data: JSON.stringify(event.candidate),
            to: id
          }
        })
      );
    }
  };
  pc.ontrack = (event) => {
    const remoteAudio = document.createElement("audio");
    remoteAudio.srcObject = event.streams[0];
    remoteAudio.volume = outputVolume.value;
    if (selectedAudioOutput.value && "setSinkId" in remoteAudio) {
      (remoteAudio as any).setSinkId(selectedAudioOutput.value).catch((error: any) => {
        console.error("扬声器设置失败:", error);
      });
    }
    remoteAudio.style.display = "none";
    remoteAudio.onended = () => {
      remoteAudio.remove();
      delete remoteAudioElements[id];
    };
    remoteAudioElements[id] = remoteAudio;
    remoteAudio.play().catch((error) => {
      console.error("Audio playback failed:", error);
    });
  };

  localStream.value!.getTracks().forEach((track) => pc.addTrack(track, localStream.value!));
  peerConnections.value[id] = pc;
  return pc;
};

const handleWebrtcAnswer = async (msg: Message) => {
  const data = JSON.parse(msg.webrtcData!.data);
  const pc = peerConnections.value[msg.webrtcData!.from];
  if (!pc) return;
  await pc.setRemoteDescription(new RTCSessionDescription(data));
};

const handleWebrtcIceCandidate = async (msg: Message) => {
  const data: RTCIceCandidateInit = JSON.parse(msg.webrtcData!.data);
  const pc = peerConnections.value[msg.webrtcData!.from];
  if (!pc) return;
  await pc.addIceCandidate(new RTCIceCandidate(data));
};

const handleElementMessage = (msg: Message) => {
  if (!isCinemaSessionActive()) return;

  console.groupCollapsed("Ws Message");
  console.log(messageTypeToJSON(msg.type));
  console.info(msg);
  console.groupEnd();
  switch (msg.type) {
    case MessageType.ERROR: {
      console.error(msg.errorMessage);
      ElNotification({
        title: "错误",
        message: msg.errorMessage,
        type: "error"
      });
      break;
    }

    case MessageType.WEBRTC_JOIN: {
      handleWebrtcJoin(msg);
      break;
    }
    case MessageType.WEBRTC_LEAVE: {
      handleWebrtcLeave(msg);
      break;
    }
    case MessageType.WEBRTC_OFFER: {
      handleWebrtcOffer(msg);
      break;
    }
    case MessageType.WEBRTC_ANSWER: {
      handleWebrtcAnswer(msg);
      break;
    }
    case MessageType.WEBRTC_ICE_CANDIDATE: {
      handleWebrtcIceCandidate(msg);
      break;
    }

    // 聊天消息
    case MessageType.CHAT: {
      if (!msg.chatContent) {
        return;
      }
      const currentTime = formatTime(new Date()); // 格式化时间
      const senderName = msg.sender?.username || "匿名用户";
      const chatContent = decodeEscapedText(msg.chatContent);
      const messageContent = `${senderName}: ${chatContent}`;
      // 添加消息到消息列表
      sendMsg({
        sender: senderName,
        content: chatContent,
        time: currentTime
      });
      sendDanmu({ text: messageContent, border: true }, player);
      break;
    }
    case MessageType.STATUS:
    case MessageType.CHECK_STATUS:
    case MessageType.SYNC: {
      switch (msg.type) {
        case MessageType.CHECK_STATUS:
          ElNotification({
            title: "状态不同步，同步中...",
            type: "warning"
          });
          break;
        case MessageType.SYNC:
          ElNotification({
            title: "同步成功",
            type: "success"
          });
          break;
      }
      setPlayerStatus(msg.playbackStatus!);
      break;
    }

    case MessageType.EXPIRED: {
      ElNotification({
        title: "链接过期,刷新中",
        type: "info"
      });
      updateSources();
      break;
    }

    // 设置正在播放的影片
    case MessageType.CURRENT: {
      getCurrentMovie();
      break;
    }

    // 播放列表更新
    case MessageType.MOVIES: {
      // 如果有权限则获取
      if (can(RoomMemberPermission.PermissionGetMovieList)) getMovies();
      break;
    }

    case MessageType.VIEWER_COUNT: {
      room.viewerCount = msg.viewerCount!;
      break;
    }

    case MessageType.MY_STATUS: {
      try {
        getMyInfo(cinemaRoomID, roomSessionID);
      } catch (err: any) {
        console.error(err);
        ElNotification({
          title: "错误",
          message: err.response.data.error || err.message,
          type: "error"
        });
        return;
      }
      break;
    }
  }
};

const noPlayArea = ref();
const playArea = ref();

// 消息区域
const chatArea = ref();

// 设置聊天框高度
const resetChatAreaHeight = () => {
  const h = playArea.value ? playArea : noPlayArea;
  if (!chatArea.value || !h.value) return;

  // 计算基础高度
  let baseHeight = h.value.scrollHeight - 112;

  // 如果有语音控制面板,减去其高度
  if (audioControls.value && audioControls.value instanceof HTMLElement) {
    baseHeight -= audioControls.value.offsetHeight + 16; // 16px for margin
  }

  chatArea.value.style.height = `${baseHeight}px`;
};

const card = ref(null);
useResizeObserver(card, resetChatAreaHeight);

const audioControls = ref<HTMLElement | null>(null);
useResizeObserver(audioControls, resetChatAreaHeight);

const can = (p: RoomMemberPermission) => {
  if (!myInfo.value) return;
  const myP = myInfo.value.permissions;
  return hasMemberPermission(myP, p);
};

const p = async () => {
  if (can(RoomMemberPermission.PermissionGetMovieList)) await getMovies();
  if (!isCinemaSessionActive()) return;
  await getCurrentMovie();
};

onMounted(async () => {
  // 获取房间信息
  try {
    await reqRoomInfoApi({
      headers: { Authorization: token.value, "X-Room-Id": cinemaRoomID }
    });
    if (!isCinemaSessionActive()) return;
  } catch (err: any) {
    if (!isCinemaSessionActive()) return;
    console.error(err);
    ElNotification({
      title: "错误",
      message: err.response.data.error || err.message,
      type: "error"
    });
    return;
  }

  // 获取用户信息
  try {
    await getMyInfo(cinemaRoomID, roomSessionID);
    if (!isCinemaSessionActive()) return;
  } catch (err: any) {
    if (!isCinemaSessionActive()) return;
    console.error(err);
    ElNotification({
      title: "错误",
      message: err.response.data.error || err.message,
      type: "error"
    });
    return;
  }

  loadChatMessages();

  // 启动websocket连接
  open();

  // 监听ws信息变化
  watchers.push(
    watch(
      () => data.value,
      async () => {
        try {
          const arr = await blobToUint8Array(data.value);
          if (!isCinemaSessionActive()) return;
          handleElementMessage(Message.decode(arr));
        } catch (err: any) {
          if (!isCinemaSessionActive()) return;
          console.error(err);
          ElMessage.error(err.message);
        }
      }
    )
  );

  await p();
  if (!isCinemaSessionActive()) return;

  // 获取初始音频设备列表
  await getAudioDevices();
  if (!isCinemaSessionActive()) return;

  // 监听设备变化
  navigator.mediaDevices.addEventListener("devicechange", getAudioDevices);
});

onBeforeUnmount(() => {
  navigator.mediaDevices.removeEventListener("devicechange", getAudioDevices);
  cleanupWebRTC();
});
</script>

<template>
  <el-row :gutter="20">
    <el-col :md="18" class="mb-5 max-sm:my-2">
      <div class="card" ref="card">
        <div
          class="card-title flex flex-wrap justify-between max-sm:text-sm max-sm:pb-4"
          v-if="playerOption.url"
        >
          <el-tag v-if="playType">{{ playType }}</el-tag>
          {{ room.currentMovie.base!.name }}
          <small class="ml-2">👁‍🗨 {{ room.viewerCount }} </small>
        </div>
        <div class="card-title flex flex-wrap justify-between max-sm:text-sm" v-else>
          当前没有影片播放，快去添加几部吧~<small class="font-normal"
            >👁‍🗨 {{ room.viewerCount }}
          </small>
        </div>
        <div class="card-body max-sm:p-0 pb-4" ref="playArea" v-if="playerOption.url">
          <div class="art-player">
            <Player @get-instance="getPlayerInstance" :options="playerOption"></Player>
          </div>
        </div>
        <div class="card-body max-sm:pb-3 max-sm:px-3" ref="noPlayArea" v-else>
          <div v-if="isLoadingCurrent" class="flex justify-center items-center h-[550px]">
            <div class="loading-spinner">
              <div class="bounce1"></div>
              <div class="bounce2"></div>
              <div class="bounce3"></div>
            </div>
          </div>
          <img v-else class="mx-auto" src="/src/assets/something-lost.webp" />
        </div>
      </div>
    </el-col>
    <el-col :md="6" class="mb-5 max-sm:mb-2">
      <div class="card h-full">
        <div class="card-title">
          在线聊天
          <el-button
            v-if="!localStream && can(RoomMemberPermission.PermissionWebRTC)"
            @click="joinWebRTC"
            type="primary"
            size="small"
            style="float: right"
            >加入语音</el-button
          >
          <el-button
            v-else-if="localStream && can(RoomMemberPermission.PermissionWebRTC)"
            @click="exitWebRTC"
            type="primary"
            size="small"
            style="float: right"
            >退出语音 ({{ peerConnectionsLengthWithUserId }})</el-button
          >
        </div>
        <div
          ref="audioControls"
          v-show="localStream"
          class="card-body mb-2 audio-controls-container"
        >
          <div class="audio-controls">
            <el-select
              v-model="selectedAudioInput"
              placeholder="选择麦克风"
              @change="switchMicrophone"
            >
              <el-option
                v-for="device in audioInputDevices"
                :key="device.deviceId"
                :label="device.label || `麦克风 ${device.deviceId.slice(0, 8)}`"
                :value="device.deviceId"
              />
            </el-select>

            <el-select
              v-model="selectedAudioOutput"
              placeholder="选择扬声器"
              @change="switchSpeaker"
            >
              <el-option
                v-for="device in audioOutputDevices"
                :key="device.deviceId"
                :label="device.label || `扬声器 ${device.deviceId.slice(0, 8)}`"
                :value="device.deviceId"
              />
            </el-select>

            <div class="volume-control">
              <span>扬声器音量:</span>
              <el-slider
                v-model="outputVolume"
                :min="0"
                :max="1"
                :step="0.1"
                @change="adjustOutputVolume"
              />
            </div>

            <el-button
              @click="toggleMute"
              :type="isMuted ? 'danger' : 'primary'"
              size="small"
              style="width: 100%"
            >
              {{ isMuted ? "取消闭麦" : "闭麦" }}
            </el-button>
          </div>
        </div>
        <div class="card-body mb-2">
          <div class="chatArea" ref="chatArea">
            <div
              class="message"
              v-for="(item, index) in chatMsgList"
              :key="`${item.time}-${index}`"
            >
              <span class="message-sender">{{ item.sender }}:</span>
              <span class="message-content">{{ item.content }}</span>
              <small v-if="item.time" class="message-time">[{{ item.time }}]</small>
            </div>
          </div>
        </div>
        <div
          v-if="can(RoomMemberPermission.PermissionSendChatMessage)"
          class="card-footer"
          style="justify-content: center; padding: 0.5rem"
        >
          <input
            type="text"
            @keyup.enter="() => sendChatText(sendChatMsg_, onSendSuccess)"
            v-model="sendChatMsg_"
            placeholder="按 Enter 键即可发送..."
            class="l-input w-full bg-transparent"
            autocomplete="off"
          />
          <button
            class="btn w-24 m-2.5 ml-0"
            @click="() => sendChatText(sendChatMsg_, onSendSuccess)"
          >
            发送
          </button>
        </div>
      </div>
    </el-col>
  </el-row>

  <el-row :gutter="20">
    <!-- 房间信息 -->
    <el-col :lg="6" :md="8" :sm="9" :xs="24" class="mb-5 max-sm:mb-2">
      <RoomInfo v-if="roomInfo" :status="status" :token="token" :roomId="roomID" :info="roomInfo" />
    </el-col>

    <!-- 影片列表 -->
    <el-col
      v-if="can(RoomMemberPermission.PermissionGetMovieList)"
      :lg="12"
      :md="16"
      :sm="15"
      :xs="24"
      class="mb-5 max-sm:mb-2"
    >
      <MovieList @send-msg="sendSystemMsg" :token="token" :roomId="roomID" />
    </el-col>

    <!-- 添加影片 -->
    <el-col
      v-if="can(RoomMemberPermission.PermissionAddMovie)"
      :lg="6"
      :md="14"
      :xs="24"
      class="mb-5 max-sm:mb-2"
    >
      <MoviePush :token="token" :roomId="roomID" />
    </el-col>
  </el-row>
</template>

<style lang="less" scoped>
.chatArea {
  overflow-y: scroll;
  height: 67vh;
  transition: height 0.3s ease;

  .message {
    overflow-wrap: anywhere;
  }

  .message-sender {
    margin-right: 0.25rem;
    font-weight: 600;
  }

  .message-time {
    margin-left: 0.35rem;
    color: #71717a;
    white-space: nowrap;
  }
}

.loading-spinner {
  margin: 0 auto;
  width: 70px;
  text-align: center;

  > div {
    width: 18px;
    height: 18px;
    background-color: #409eff;
    border-radius: 100%;
    display: inline-block;
    animation: bounce 1.4s infinite ease-in-out both;
    margin: 0 3px;
  }

  .bounce1 {
    animation-delay: -0.32s;
  }

  .bounce2 {
    animation-delay: -0.16s;
  }
}

.audio-controls-container {
  transition: all 0.3s ease-in-out;
  transform-origin: top;
  animation: slideDown 0.3s ease-in-out;
}

.audio-controls {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin-bottom: 10px;

  .el-select {
    width: 100%;
    animation: fadeIn 0.3s ease-in-out;
  }

  .volume-control {
    animation: fadeIn 0.3s ease-in-out 0.1s;
  }

  .el-button {
    animation: fadeIn 0.3s ease-in-out 0.2s;
  }
}

@keyframes slideDown {
  from {
    transform: scaleY(0);
    opacity: 0;
  }
  to {
    transform: scaleY(1);
    opacity: 1;
  }
}

@keyframes fadeIn {
  from {
    opacity: 0;
    transform: translateY(-10px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@keyframes bounce {
  0%,
  80%,
  100% {
    transform: scale(0);
  }
  40% {
    transform: scale(1);
  }
}
</style>
