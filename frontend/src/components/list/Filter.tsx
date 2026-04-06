import React, {useEffect, useState} from "react";
import {Button, DatePicker, Input, InputNumber, Radio, Select} from "antd";
import type {Dayjs} from "dayjs";

export type ReviewFilterValues = {
    title: string;
    type?: string;
    status?: boolean;
    partyA?: string;
    partyB?: string;
    dateRange: [Dayjs | null, Dayjs | null] | null;
};

export type FilterType = 'Review' | 'Contrast';

type FilterProps = {
    filters: ReviewFilterValues;
    onFiltersChange: (changed: Partial<ReviewFilterValues>) => void;
    onReset: () => void;
    onSearch: () => void;
    typeOptions: { value: string; label: string }[];
    type?: FilterType;
    onTypeChange?: (type: FilterType) => void;
};
export default function Filter({
                                   filters,
                                   onFiltersChange,
                                   onReset,
                                   onSearch,
                                   typeOptions,
                                   type: controlledType,
                                   onTypeChange
                               }: FilterProps) {
    const {RangePicker} = DatePicker;
    const [type, setType] = useState<FilterType>(controlledType ?? "Review");

    useEffect(() => {
        if (controlledType && controlledType !== type) {
            setType(controlledType);
        }
    }, [controlledType, type]);

    const handleTypeChange = (next: FilterType) => {
        setType(next);
        onTypeChange?.(next);
    };
    return (
        <>
            <div className="ml-[1.25rem] mb-[1.25rem] h-[2rem]">
                <Radio.Group
                    value={type}
                    onChange={(e) => handleTypeChange(e.target.value as FilterType)}
                    optionType="button"
                    buttonStyle="solid"
                >
                    <Radio.Button value="Review">合同审阅</Radio.Button>
                    <Radio.Button value="Contrast">合同比对</Radio.Button>
                </Radio.Group>
            </div>
            {type === "Review" ? (
                <div className="flex items-center mx-[1.25rem] bg-white h-[3.63rem] rounded-[0.31rem]">
                    <div className="flex items-center flex-1">
                        <label className="text-[0.88rem] text-black ml-[0.75rem] whitespace-nowrap">合同名称：</label>
                        <Input
                            placeholder="请输入合同名称"
                            className="!w-[13.81rem]"
                            value={filters.title}
                            onChange={(e) => onFiltersChange({title: e.target.value})}
                        />
                    </div>
                    <div className="flex items-center flex-1">
                        <label className="text-[0.88rem] text-black whitespace-nowrap">合同类型：</label>
                        <Select
                            placeholder="请选择"
                            options={typeOptions}
                            allowClear
                            className="w-[10.69rem]"
                            value={filters.type}
                            onChange={(value) => onFiltersChange({type: value ?? undefined})}
                        />
                    </div>
                    <div className="flex items-center flex-1">
                        <label className="text-[0.88rem] text-black whitespace-nowrap">修订状态：</label>
                        <Select
                            placeholder="请选择"
                            options={[
                                {value: true, label: "已修订"},
                                {value: false, label: "未修订"}
                            ]}
                            allowClear
                            className="w-[10.69rem]"
                            value={filters.status}
                            onChange={(value) => onFiltersChange({status: value})}
                        />
                    </div>
                    <div className="flex items-center flex-1">
                        <label className="text-[0.88rem] text-black whitespace-nowrap">创建时间：</label>
                        <RangePicker
                            showTime
                            placeholder={['开始时间', '结束时间']}
                            className="w-[13.13rem]"
                            value={filters.dateRange}
                            onChange={(value) => onFiltersChange({dateRange: value})}
                        />
                    </div>
                    <div className="flex items-center ml-auto mr-[1.75rem]">
                        <Button className="w-[3.75rem]  mr-[0.88rem]" onClick={onReset}>重置</Button>
                        <Button className="w-[3.75rem] " type="primary" onClick={onSearch}>查询</Button>
                    </div>
                </div>
            ) : (
                <div className="flex items-center mx-[1.25rem] bg-white h-[3.63rem] rounded-[0.31rem]">
                    <div className="flex items-center flex-1">
                        <label className="text-[0.88rem] text-black ml-[0.75rem] whitespace-nowrap">合同名称：</label>
                        <Input
                            placeholder="请输入合同名称"
                            className="!w-[13.81rem]"
                            value={filters.title}
                            onChange={(e) => onFiltersChange({title: e.target.value})}
                        />
                    </div>
                    <div className="flex items-center flex-1">
                        <label className="text-[0.88rem] text-black whitespace-nowrap">相似度：</label>
                        <InputNumber
                            className="!w-[8rem]"
                            placeholder="请输入下限"
                        />
                        <span className='w-[0.75rem] h-[0.13rem] bg-[#CCCCCC] items-center mx-[0.5rem]'></span>
                        <InputNumber
                            className="!w-[8rem]"
                            placeholder="请输入上限"
                        />
                    </div>
                    <div className="flex items-center flex-1">
                        <label className="text-[0.88rem] text-black whitespace-nowrap">审核状态：</label>
                        <Select
                            placeholder="请选择"
                            options={[
                                {value: true, label: "已修订"},
                                {value: false, label: "未修订"}
                            ]}
                            allowClear
                            className="w-[10.69rem]"
                            value={filters.status}
                            onChange={(value) => onFiltersChange({status: value})}
                        />
                    </div>
                    <div className="flex items-center flex-1">
                        <label className="text-[0.88rem] text-black whitespace-nowrap">任务创建时间：</label>
                        <RangePicker
                            showTime
                            placeholder={['开始时间', '结束时间']}
                            className="w-[13.13rem]"
                            value={filters.dateRange}
                            onChange={(value) => onFiltersChange({dateRange: value})}
                        />
                    </div>
                    <div className="flex items-center mr-[1.75rem]">
                        <Button className="w-[3.75rem] ml-auto mr-[0.88rem]" onClick={onReset}>重置</Button>
                        <Button className="w-[3.75rem]" type="primary" onClick={onSearch}>查询</Button>
                    </div>
                </div>)}
        </>
    )
}