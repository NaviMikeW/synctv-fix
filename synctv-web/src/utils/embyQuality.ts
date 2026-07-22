export interface EmbyQualityOption {
  name: string;
  bitrate: number;
  url: string;
}

const QUALITY_PRESETS = [
  { name: "超清 16 Mbps", bitrate: 16_000_000 },
  { name: "高清 8 Mbps", bitrate: 8_000_000 },
  { name: "标清 4 Mbps", bitrate: 4_000_000 },
  { name: "流畅 2 Mbps", bitrate: 2_000_000 }
];

const DEVICE_STORAGE_KEY = "synctv-emby-device-id";

function randomID(): string {
  try {
    if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
      return crypto.randomUUID().replaceAll("-", "");
    }
    if (typeof crypto !== "undefined" && typeof crypto.getRandomValues === "function") {
      const bytes = new Uint8Array(16);
      crypto.getRandomValues(bytes);
      return Array.from(bytes, (value) => value.toString(16).padStart(2, "0")).join("");
    }
  } catch {
    // Fall through to the non-cryptographic compatibility fallback.
  }
  return `${Date.now().toString(16)}${Math.random().toString(16).slice(2)}`
    .padEnd(32, "0")
    .slice(0, 32);
}

function readOrCreate(storage: Storage | undefined, key: string): string {
  if (!storage) return randomID();
  try {
    const value = storage.getItem(key);
    if (value) return value;
    const created = randomID();
    storage.setItem(key, created);
    return created;
  } catch {
    return randomID();
  }
}

function browserStorage(type: "local" | "session"): Storage | undefined {
  if (typeof window === "undefined") return undefined;
  try {
    return type === "local" ? window.localStorage : window.sessionStorage;
  } catch {
    return undefined;
  }
}

function isEmbyMasterPlaylist(url: URL): boolean {
  const path = url.pathname.toLowerCase();
  return path.includes("/emby/videos/") && path.endsWith("/master.m3u8");
}

function sessionKey(url: URL, bitrate: number): string {
  const itemMatch = url.pathname.match(/\/emby\/videos\/([^/]+)/i);
  const itemID = itemMatch?.[1] || url.pathname;
  return `synctv-emby-play-session:${itemID}:${bitrate}`;
}

function makeVariant(base: URL, bitrate: number): string {
  const variant = new URL(base.toString());
  variant.searchParams.set("VideoBitrate", String(bitrate));

  if (variant.searchParams.has("MaxStreamingBitrate")) {
    variant.searchParams.set("MaxStreamingBitrate", String(bitrate));
  }

  const deviceID = readOrCreate(browserStorage("local"), DEVICE_STORAGE_KEY);
  const playSessionID = readOrCreate(browserStorage("session"), sessionKey(base, bitrate));
  variant.searchParams.set("DeviceId", deviceID);
  variant.searchParams.set("PlaySessionId", playSessionID);
  return variant.toString();
}

export function buildEmbyQualityOptions(rawURL: string): EmbyQualityOption[] {
  if (!rawURL) return [];

  let base: URL;
  try {
    base = new URL(rawURL, typeof window === "undefined" ? "http://localhost" : window.location.href);
  } catch {
    return [];
  }

  if (!isEmbyMasterPlaylist(base)) return [];

  const currentBitrate = Number(base.searchParams.get("VideoBitrate"));
  if (!Number.isFinite(currentBitrate) || currentBitrate <= 0) return [];

  const presets = [...QUALITY_PRESETS];
  const hasNearCurrentPreset = presets.some(
    (preset) => Math.abs(preset.bitrate - currentBitrate) / currentBitrate < 0.08
  );
  if (!hasNearCurrentPreset) {
    presets.push({
      name: `当前 ${(currentBitrate / 1_000_000).toFixed(1)} Mbps`,
      bitrate: Math.round(currentBitrate)
    });
  }

  const seen = new Set<number>();
  return presets
    .sort((left, right) => right.bitrate - left.bitrate)
    .filter((preset) => {
      if (seen.has(preset.bitrate)) return false;
      seen.add(preset.bitrate);
      return true;
    })
    .map((preset) => ({
      ...preset,
      url: makeVariant(base, preset.bitrate)
    }));
}

export function embyVideoBitrate(rawURL: string): number | undefined {
  try {
    const url = new URL(rawURL, typeof window === "undefined" ? "http://localhost" : window.location.href);
    const value = Number(url.searchParams.get("VideoBitrate"));
    return Number.isFinite(value) && value > 0 ? value : undefined;
  } catch {
    return undefined;
  }
}
