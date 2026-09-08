import { toast } from 'sonner'

/** document.execCommand is deprecated but still the only option over plain HTTP. */
function fallbackCopy(text: string) {
  const area = document.createElement('textarea')
  area.value = text
  area.setAttribute('readonly', '')
  area.style.position = 'fixed'
  area.style.opacity = '0'
  document.body.appendChild(area)
  area.select()
  document.execCommand('copy')
  document.body.removeChild(area)
}

export async function copyToClipboard(text: string, label = 'Kopiert') {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text)
    } else {
      fallbackCopy(text)
    }
    toast.success(label)
  } catch {
    toast.error('Kopieren fehlgeschlagen — bitte manuell markieren')
  }
}
