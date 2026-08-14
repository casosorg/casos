import i18next from "i18next";

export const localPasswordErrors = {
  tooShort: "Password must contain at least 8 characters",
  tooLong: "Password cannot be longer than 72 bytes",
};

export function getLocalPasswordError(password) {
  if (!password || Array.from(password).length < 8) {
    return localPasswordErrors.tooShort;
  }
  if (new TextEncoder().encode(password).length > 72) {
    return localPasswordErrors.tooLong;
  }
  return "";
}

export function getLocalPasswordErrorMessage(password) {
  const error = getLocalPasswordError(password);
  if (error === localPasswordErrors.tooShort) {
    return i18next.t("account:Password must contain at least 8 characters");
  }
  if (error === localPasswordErrors.tooLong) {
    return i18next.t("account:Password cannot be longer than 72 bytes");
  }
  return "";
}
