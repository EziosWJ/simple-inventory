import type { DictSelectOption } from "@/constants/dicts";

type DictValue = string | number;

export function businessDictLabel(
  options: readonly DictSelectOption<DictValue>[],
  value: DictValue | boolean | null | undefined,
) {
  if (value === null || value === undefined || value === "") return "-";
  return options.find((option) => String(option.value) === String(value))?.label
    ?? `${value}（字典项缺失）`;
}

export function preserveCurrentDictOption<T extends DictValue>(
  options: readonly DictSelectOption<T>[],
  value: T,
): DictSelectOption<T>[] {
  if (options.some((option) => String(option.value) === String(value))) {
    return [...options];
  }
  return [...options, { value, label: `${value}（字典项缺失）` }];
}

export function missingBusinessDictValues<T extends DictValue>(
  options: readonly DictSelectOption<T>[],
  expected: readonly T[],
) {
  return expected.filter(
    (value) => !options.some((option) => String(option.value) === String(value)),
  );
}
