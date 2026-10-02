export function PrintHeading({ title, name, phone, address }: {
  title: string; name: string; phone: string; address?: string;
}) {
  const initial = Array.from(name.trim())[0];
  return (
    <header className="delivery-head">
      <h1 className="delivery-title">{title}</h1>
      <div className="delivery-owner">
        {initial && <span className="delivery-owner-mark" aria-label="公司名称首字标识">{initial}</span>}
        <div className="delivery-owner-details">
          <strong>{name}</strong>
          <span>联系电话：{phone}</span>
          {address && <span>经营地址：{address}</span>}
        </div>
      </div>
    </header>
  );
}
