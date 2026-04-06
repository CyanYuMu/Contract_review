export const signboardMock = {
    // 中间数据卡片数据
    contractReviewData: {
        reviewedCount: 266,
        useDeptCount: 20,
        usePersonCount: 266,
        ServiceCount: 266,
        budgetCount: 266,
        infrastructrueCount: 266
    },

    // 折线图数据（合同需求趋势）
    trendChart: {
        xAxis: ['1', '2', '3', '4', '5', '6', '7', '8', '9', '10', '11', '12'],
        yAxisData1: [1700, 2200, 600, 1400, 2500, 2200, 800, 1200, 1000, 900, 1000],// 模拟趋势数值
        yAxisData2: [500, 700, 500, 400, 900, 700, 600, 500, 300, 500, 300, 200] // 模拟趋势数值

    },
    // 右侧企业使用情况表格
    DepartmentUsageItem: [
        {department_name: "后勤处", contract_review: 1, contract_verification: 22, contract_comparison: 33, total: 67},
        {department_name: "基建处", contract_review: 12, contract_verification: 22, contract_comparison: 33, total: 67},
        {department_name: "学生处", contract_review: 2, contract_verification: 2, contract_comparison: 2, total: 6},
        {department_name: "招生就业处", contract_review: 1, contract_verification: 1, contract_comparison: 1, total: 3},
        {
            department_name: "信息中心/信息化办",
            contract_review: 1,
            contract_verification: 1,
            contract_comparison: 1,
            total: 3
        },
        {department_name: "后勤处", contract_review: 1, contract_verification: 1, contract_comparison: 1, total: 3},
        {department_name: "后勤处", contract_review: 1, contract_verification: 1, contract_comparison: 1, total: 3},
        {department_name: "后勤处", contract_review: 1, contract_verification: 1, contract_comparison: 1, total: 3},
        {department_name: "后勤处", contract_review: 1, contract_verification: 1, contract_comparison: 1, total: 3},
        {department_name: "后勤处", contract_review: 1, contract_verification: 1, contract_comparison: 1, total: 3},
        {department_name: "后勤处", contract_review: 1, contract_verification: 1, contract_comparison: 1, total: 3},
    ],
    wordCloudData: [
        {name: '条款引用错误', value: 100},
        {name: '货物数目错误', value: 80},
        {name: '条款描述错误', value: 70},
        {name: '称呼错误', value: 60},
        {name: '指代不明', value: 50},
        {name: '金额计算错误', value: 90},
        {name: '日期格式错误', value: 40},
        {name: '签字盖章缺失', value: 85},
        {name: '合同主体错误', value: 95},
        {name: '风险点', value: 65},
        {name: '错误点', value: 55},
        {name: '审阅', value: 75},
    ]
};
export const generateMockYesterdayData = () => {
    // 1. 获取今日的真实数据
    const todayData = signboardMock.contractReviewData;

    // 2. 定义一个辅助函数，用于生成接近某个数值的随机数
    const getRandomCloseValue = (value: number) => {
        // 生成一个 0.9 到 1.1 之间的随机乘数
        // Math.random() 生成 0 到 1 之间的随机数
        // Math.random() * 0.2 生成 0 到 0.2 之间的随机数
        // 0.9 + Math.random() * 0.2 最终生成 0.9 到 1.1 之间的随机数
        const randomFactor = 0.9 + Math.random() * 0.2;

        // 将今日的值乘以这个随机乘数，得到一个在 ±10% 范围内波动的数值
        // Math.round() 用于将结果四舍五入为整数，因为合同数量等指标通常是整数
        return Math.round(value * randomFactor);
    };

    // 3. 使用辅助函数为每个指标生成昨日数据
    return {
        reviewedCount: getRandomCloseValue(todayData.reviewedCount),
        useDeptCount: getRandomCloseValue(todayData.useDeptCount),
        usePersonCount: getRandomCloseValue(todayData.usePersonCount),
        ServiceCount: getRandomCloseValue(todayData.ServiceCount),
        budgetCount: getRandomCloseValue(todayData.budgetCount),
        infrastructrueCount: getRandomCloseValue(todayData.infrastructrueCount),
    };
};

export const contrastMock = [
    {
        id: 1,
        origin_contract_name: "合同变更标准合同名称.docx",
        new_contract_name: "合同变更比对合同名称.docx",
        similarity: 71.1,
        status: false, // 未审核
        dateRange: "2025/12/01 00:00:02",
    },
    {
        id: 2,
        origin_contract_name: "合同变更标准合同名称.docx",
        new_contract_name: "合同变更比对合同名称.docx",
        similarity: 71.1,
        status: false,
        dateRange: "2025/12/01 00:00:02",
    },
    {
        id: 3,
        origin_contract_name: "合同变更标准合同名称.docx",
        new_contract_name: "合同变更比对合同名称.docx",
        similarity: 91.1,
        status: true, // 已审核
        dateRange: "2025/12/01 00:00:02",
    },
    {
        id: 4,
        origin_contract_name: "合同变更标准合同名称.docx",
        new_contract_name: "合同变更比对合同名称.docx",
        similarity: 71.1,
        status: false,
        dateRange: "2025/12/01 00:00:02",
    },
    {
        id: 5,
        origin_contract_name: "合同变更标准合同名称.docx",
        new_contract_name: "合同变更比对合同名称.docx",
        similarity: 1.1,
        status: true,
        dateRange: "2025/12/01 00:00:02",
    },
    {
        id: 6,
        origin_contract_name: "合同变更标准合同名称.docx",
        new_contract_name: "合同变更比对合同名称.docx",
        similarity: 71.1,
        status: false,
        dateRange: "2025/12/01 00:00:02",
    },
    {
        id: 7,
        origin_contract_name: "合同变更标准合同名称.docx",
        new_contract_name: "合同变更比对合同名称.docx",
        similarity: 71.1,
        status: false,
        dateRange: "2025/12/01 00:00:02",
    },
    {
        id: 8,
        origin_contract_name: "合同变更标准合同名称.docx",
        new_contract_name: "合同变更比对合同名称.docx",
        similarity: 71.1,
        status: false,
        dateRange: "2025/12/01 00:00:02",
    },
    {
        id: 9,
        origin_contract_name: "合同变更标准合同名称.docx",
        new_contract_name: "合同变更比对合同名称.docx",
        similarity: 71.1,
        status: false,
        dateRange: "2025/12/01 00:00:02",
    },
];
