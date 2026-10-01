import { http } from "@/lib/http";

export type PrintProfile = { name: string; phone: string; address: string };

export function getPrintProfile() {
  return http.get<PrintProfile>("/api/v1/print-profile");
}

export function updatePrintProfile(profile: PrintProfile) {
  return http.put<PrintProfile>("/api/v1/print-profile", profile);
}
