#!/bin/bash
# Testa o instalador AutoPecas-Setup no Wine, capturando a tela
set -u
export WINEPREFIX=/home/user/.cache/wineprefix
export WINEDEBUG=-all
DISP=:77
rm -f /home/user/qa/instalador-1.png /home/user/qa/instalador-2.png
Xvfb $DISP -screen 0 1024x700x24 >/dev/null 2>&1 &
XVFB_PID=$!
sleep 2
export DISPLAY=$DISP
cd /home/user/dist
/usr/lib/wine/wine64 ./AutoPecas-Setup-1.0.0.exe >/home/user/qa/wine-setup.log 2>&1 &
sleep 12
xdotool search --name "Instalacao do AutoPecas" | head -1 > /tmp/winid
echo "janela: $(cat /tmp/winid)"
xwd -root -display $DISP 2>/dev/null | convert xwd:- /home/user/qa/instalador-1.png
echo "print 1 salvo"
WID=$(cat /tmp/winid)
xdotool windowactivate $WID 2>/dev/null; sleep 1
xdotool key --window $WID Return
sleep 6
xwd -root -display $DISP 2>/dev/null | convert xwd:- /home/user/qa/instalador-2.png
echo "print 2 salvo"
kill $XVFB_PID 2>/dev/null
pkill -f AutoPecas-Setup 2>/dev/null
sleep 1
echo "--- pastas instaladas:"
ls -la "$HOME/.wine/drive_c/users/"*/AppData/Local/Programs/AutoPecas 2>/dev/null || find /home/user/.cache/wineprefix -iname "AutoPecas.exe" 2>/dev/null | head -5
