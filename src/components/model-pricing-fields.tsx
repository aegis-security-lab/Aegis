import {
  Field,
  FieldDescription,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import type { ModelPricing } from "@/types"

const priceFields: Array<{
  key: keyof ModelPricing
  label: string
}> = [
  { key: "input", label: "输入" },
  { key: "output", label: "输出" },
  { key: "cacheRead", label: "缓存读取" },
  { key: "cacheWrite", label: "缓存写入" },
]

export function ModelPricingFields({
  idPrefix,
  value,
  onChange,
}: {
  idPrefix: string
  value: ModelPricing
  onChange: (value: ModelPricing) => void
}) {
  return (
    <FieldSet>
      <FieldLegend>模型价格</FieldLegend>
      <FieldDescription>
        单位为 USD / 100 万 tokens，Execution 创建时会冻结一份价格快照。
      </FieldDescription>
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        {priceFields.map((field) => {
          const id = `${idPrefix}-${field.key}`
          return (
            <Field key={field.key}>
              <FieldLabel htmlFor={id}>{field.label}</FieldLabel>
              <Input
                id={id}
                type="number"
                min="0"
                step="0.000001"
                inputMode="decimal"
                value={value[field.key]}
                onChange={(event) =>
                  onChange({
                    ...value,
                    [field.key]: Number.isFinite(event.target.valueAsNumber)
                      ? event.target.valueAsNumber
                      : 0,
                  })
                }
              />
            </Field>
          )
        })}
      </div>
    </FieldSet>
  )
}
