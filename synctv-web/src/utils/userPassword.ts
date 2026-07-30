export const USER_PASSWORD_MIN_LENGTH = 12;
export const USER_PASSWORD_MAX_LENGTH = 32;
export const USER_PASSWORD_POLICY_MESSAGE =
  "密码应为 12 ~ 32 位 ASCII 可打印字符，不能使用中文或 Emoji";
export const USER_PASSWORD_PRINTABLE_ASCII_PATTERN = /^[\x20-\x7e]+$/;

export const isValidNewUserPassword = (value: string): boolean =>
  value.length >= USER_PASSWORD_MIN_LENGTH &&
  value.length <= USER_PASSWORD_MAX_LENGTH &&
  USER_PASSWORD_PRINTABLE_ASCII_PATTERN.test(value);
