import { Link, useSearchParams } from "react-router-dom";
import { businessReturnTo } from "@/lib/business-navigation";
export function BusinessReturnLink(){const [params]=useSearchParams();const path=businessReturnTo(params.get("returnTo"),"");return path ? <div className="print-hide mb-space-4"><Link to={path} className="text-sm text-primary underline">返回来源页面</Link></div>:null;}
