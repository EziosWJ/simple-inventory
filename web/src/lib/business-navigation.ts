// Only local business routes can be used as a return destination.
export function businessReturnTo(value:string|null, fallback:string) {
  if(!value || !value.startsWith("/business/") || value.includes("\\")) return fallback;
  try {const url=new URL(value,"http://local");return url.origin==="http://local" ? url.pathname+url.search : fallback;}catch{return fallback;}
}
export function withBusinessReturn(path:string, from:string) {
  const [pathname, search=""] = path.split("?");
  const params=new URLSearchParams(search);params.set("returnTo",from);return `${pathname}?${params}`;
}
