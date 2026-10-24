// toFixed 修复版： 修复 1.335.toFixed(2) => 1.33
export function toFixed(val: number, s = 2) {
  const num = val || 0
  if (/^\d+$/.test(String(num))) {
    // 整数
    return num.toFixed(s)
  }
  const times = Math.pow(10, s)
  let des = num * times + 0.5
  des = parseInt(String(des), 10) / times
  return des + ''
}

export function loadScript(url: string) {
  // 检查脚本是否已经加载
  const node = document.head.querySelector(`script[src="${url}"]`)
  if (node) {
    console.log(`脚本(${url})已加载`)
    return Promise.resolve(node)
  }
  return new Promise((resolve, reject) => {
    const script = document.createElement('script')
    script.type = 'text/javascript'
    script.onload = () => {
      console.log(`脚本(${url})加载完成`)
      resolve(script)
    }
    script.onerror = reject
    script.src = url
    document.head.appendChild(script)
  })
}

export function removeScript(url: string) {
  const node = document.head.querySelector(`script[src="${url}"]`)
  if (node) {
    document.head.removeChild(node)
  }
}
